package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// guard_restart_test.go pins the branch guards' statement islands
// (landing_restart.go, NUR292): which guards a plan takes, the paren the
// island writes the guarded value in place of, what a second run may repeat,
// and how the lowering seats and emits one. The lang suite drives the
// planners end to end (TestNUR292StatementIslandRunsTheList).

func gpos(col int) core.SrcPos { return core.SrcPos{Row: 1, Col: col} }

func gtok(v core.Value, col int) core.Value {
	v.SetPos(gpos(col))
	return v
}

// guardBody is `w end 5 if (mk) ["t"] [(mk) 1]`: the statement begins at
// token 2, the condition's paren is token 4 (mk at column 11), and the
// else arm is a list literal holding a paren (mk at column 22).
func guardBody() []core.Value {
	paren := gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 11)}), 10)
	inner := gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 22)}), 21)
	els := gtok(core.NewEvalList([]core.Value{inner, gtok(core.NewInteger(1), 26)}), 20)
	return []core.Value{
		gtok(core.NewWord("w"), 1), gtok(core.NewEnd(), 3), gtok(core.NewInteger(5), 5), gtok(core.NewWord("if"), 7),
		paren, gtok(core.NewEvalList([]core.Value{gtok(core.NewString("t"), 16)}), 15), els,
	}
}

func TestBranchGuardsAndKeys(t *testing.T) {
	br := &emitBranch{condGuard: true, thenGuard: true, elsGuard: true, cond: EmitOperand{kind: opLocal, idx: 1}, thenVal: EmitOperand{kind: opConst}, elsVal: EmitOperand{kind: opEvent, idx: 2}}
	tree := map[int]treeEvent{
		7: {ev: &EmitEvent{kind: evBranch, seq: 7, br: br}},
		3: {ev: &EmitEvent{kind: evBranch, seq: 3, br: &emitBranch{thenGuard: true}}, inLoop: true},
		4: {ev: &EmitEvent{kind: evBranch, seq: 4, br: &emitBranch{}}},
		5: {ev: &EmitEvent{kind: evCall, seq: 5}},
	}
	got := branchGuards(tree)
	if len(got) != 4 || got[0].seq != 3 || got[1].kind != guardCond || got[2].kind != guardThen || got[3].kind != guardElse || got[3].op.idx != 2 {
		t.Errorf("every branch lists its guarded operands in order, a loop's too: %+v", got)
	}
	if guardKey(7, guardCond) == guardKey(7, guardThen) || guardKey(7, guardElse) == guardKey(8, guardCond) {
		t.Error("guardKey keys each guard of each branch apart")
	}
}

func TestRestartAnchorAndSeated(t *testing.T) {
	if got := restartAnchor(&EmitEvent{kind: evBranch, br: &emitBranch{pos: gpos(3), condCheckPos: gpos(9)}}); got != gpos(9) {
		t.Errorf("a guarded branch stands at its `if` word: %v", got)
	}
	if got := restartAnchor(&EmitEvent{kind: evBranch, br: &emitBranch{pos: gpos(3)}}); got != gpos(3) {
		t.Errorf("another branch stands at its own position: %v", got)
	}
	if got := restartAnchor(&EmitEvent{kind: evCall, call: emitCall{pos: gpos(4)}}); got != gpos(4) {
		t.Errorf("a call stands at its own position: %v", got)
	}
	var none *landingRestart
	for _, c := range []struct {
		r    *landingRestart
		want bool
	}{
		{none, false},
		{&landingRestart{depth: -1, held: -1}, false},
		{&landingRestart{depth: 0, held: -1, unseatable: true}, false},
		{&landingRestart{depth: 1, held: 0}, false},
		{&landingRestart{depth: 1, held: 1}, true},
		{&landingRestart{depth: 2, held: -1}, true},
	} {
		if got := c.r.seated(); got != c.want {
			t.Errorf("seated(%+v) = %v, want %v", c.r, got, c.want)
		}
	}
}

func TestTokenPathAndNestedToks(t *testing.T) {
	body := guardBody()
	if got := tokenPath(body, gpos(22)); len(got) != 3 || got[0] != 6 || got[1] != 0 || got[2] != 0 {
		t.Errorf("a position inside the else arm's paren: %v", got)
	}
	if got := tokenPath(body, core.SrcPos{}); len(got) != 0 {
		t.Errorf("a position no token holds has no path: %v", got)
	}
	quoted := core.NewEvalList([]core.Value{core.NewInteger(1)})
	quoted.Quoted = true
	for _, c := range []struct {
		name string
		v    core.Value
		want bool
	}{
		{"a paren", body[4], true},
		{"a list literal", body[5], true},
		{"a quoted list", quoted, false},
		{"a computed list", core.NewList(nil), false},
		{"a word", body[0], false},
	} {
		if _, got := nestedToks(c.v); got != c.want {
			t.Errorf("%s: nestedToks = %v, want %v", c.name, got, c.want)
		}
	}
	if hasPrefix([]int{1, 2}, nil) || hasPrefix([]int{1}, []int{1, 2}) || hasPrefix([]int{1, 3}, []int{1, 2}) || !hasPrefix([]int{1, 2, 0}, []int{1, 2}) {
		t.Error("hasPrefix")
	}
}

func TestCallShape(t *testing.T) {
	ops := []EmitOperand{{kind: opConst}}
	if o, n := callShape(&EmitEvent{kind: evCall, call: emitCall{ops: ops, nout: 2}}); len(o) != 1 || n != 2 {
		t.Errorf("a native call's shape: %v %d", o, n)
	}
	if o, n := callShape(&EmitEvent{kind: evCallUser, uc: emitUserCall{ops: ops, nout: 1}}); len(o) != 1 || n != 1 {
		t.Errorf("a user call's shape: %v %d", o, n)
	}
	if o, n := callShape(&EmitEvent{kind: evBranch}); o != nil || n != 0 {
		t.Errorf("no call, no shape: %v %d", o, n)
	}
}

func TestParenOf(t *testing.T) {
	body := guardBody()
	mk := func(col, nout int, ops ...EmitOperand) *EmitEvent {
		return &EmitEvent{kind: evCallUser, uc: emitUserCall{pos: gpos(col), nout: nout, ops: ops}}
	}
	for _, c := range []struct {
		name string
		ev   *EmitEvent
		path []int
		ok   bool
	}{
		{"the condition's paren", mk(11, 1), []int{4}, true},
		{"a paren inside the else arm", mk(22, 1), []int{6, 0}, true},
		{"a paren that leaves two values", mk(11, 2), []int{4}, false},
		{"a call that took no token for its operand", mk(11, 1, EmitOperand{kind: opConst}), []int{4}, false},
		{"a bare word whose call took nothing", mk(7, 1), []int{3}, true},
		{"a bare word whose call took an operand", mk(7, 1, EmitOperand{kind: opConst}), []int{3}, false},
		{"an earlier statement's call", mk(1, 1), nil, false},
		{"a position-less call", &EmitEvent{kind: evCallUser, uc: emitUserCall{nout: 1}}, nil, false},
	} {
		path, ok := parenOf(c.ev, body, 2)
		if ok != c.ok || len(path) != len(c.path) || (len(path) > 0 && !hasPrefix(path, c.path)) {
			t.Errorf("%s: parenOf = %v %v, want %v %v", c.name, path, ok, c.path, c.ok)
		}
	}
}

func TestRestartSubsts(t *testing.T) {
	body := guardBody()
	call := func(seq, col int) EmitEvent {
		return EmitEvent{kind: evCallUser, seq: seq, uc: emitUserCall{pos: gpos(col), nout: 1}}
	}
	read := EmitEvent{kind: evCall, seq: 3, call: emitCall{word: "dot", pos: gpos(5)}}
	inner := EmitEvent{kind: evCallUser, seq: 4, uc: emitUserCall{pos: gpos(22), nout: 1}}
	outer := EmitEvent{kind: evCallUser, seq: 5, uc: emitUserCall{pos: gpos(20), nout: 1}}
	tree := rootTreeEvents([]EmitEvent{read, call(6, 11), call(7, 22), inner, outer}, false)
	got, ok := NewEmitState().restartSubsts(tree, body, 2, []int{3, 6, 7})
	if !ok || len(got) != 2 || got[0].seq != 6 || got[1].seq != 7 || len(got[1].path) != 2 {
		t.Errorf("a read runs again, and each call's paren is written as its value: %+v %v", got, ok)
	}
	// A paren opened by a call nested in a substituted one is its outer
	// paren's; the outermost is written.
	nested := map[int]treeEvent{
		1: {ev: &EmitEvent{kind: evCallUser, seq: 1, uc: emitUserCall{pos: gpos(11), nout: 1}}},
		2: {ev: &EmitEvent{kind: evCallUser, seq: 2, uc: emitUserCall{pos: gpos(12), nout: 1}}},
	}
	deep := []core.Value{gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("f"), 11), gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("g"), 12)}), 12)}), 10)}
	nested[1].ev.uc.ops = []EmitOperand{{kind: opEvent, idx: 2}}
	if got, ok := NewEmitState().restartSubsts(nested, deep, 0, []int{1, 2}); !ok || len(got) != 1 || got[0].seq != 1 {
		t.Errorf("the outermost paren is written: %+v %v", got, ok)
	}
	bind := map[int]treeEvent{1: {ev: &EmitEvent{kind: evCallUser, seq: 1, uc: emitUserCall{pos: gpos(11), nout: 1}}}, 2: {ev: &EmitEvent{kind: evDynBind, seq: 2, dyn: &emitDynBind{pos: gpos(11)}}}}
	if _, ok := NewEmitState().restartSubsts(bind, body, 2, []int{1, 2}); ok {
		t.Error("a bind inside a written paren is one the island would miss")
	}
	effect := map[int]treeEvent{1: {ev: &EmitEvent{kind: evCall, seq: 1, call: emitCall{word: "print", pos: gpos(5)}}}}
	if _, ok := NewEmitState().restartSubsts(effect, body, 2, []int{1}); ok {
		t.Error("an effect outside any written paren would run twice")
	}
}

// guardTree is the tree of guardBody's branch (seq 10, its condition guarded)
// with the given events beside it: a then arm holding a print (seq 8), and
// the condition's call (seq 6) inside the paren.
func guardTree(extra ...EmitEvent) map[int]treeEvent {
	then := &EmitFragment{events: []EmitEvent{{kind: evCall, seq: 8, call: emitCall{word: "print", pos: gpos(16)}}}}
	events := []EmitEvent{
		{kind: evCall, seq: 1, call: emitCall{word: "print", pos: gpos(1)}},
		{kind: evCallUser, seq: 6, uc: emitUserCall{pos: gpos(11), nout: 1}},
		{kind: evBranch, seq: 10, br: &emitBranch{condGuard: true, cond: EmitOperand{kind: opEvent, idx: 6}, then: then, pos: gpos(11), condCheckPos: gpos(7)}},
		{kind: evCall, seq: 12, call: emitCall{word: "print", pos: gpos(30)}},
	}
	return rootTreeEvents(append(events, extra...), false)
}

func TestGuardReruns(t *testing.T) {
	body := guardBody()
	substs, ok := NewEmitState().guardReruns(guardTree(), 10, body, 2)
	if !ok || len(substs) != 1 || substs[0].seq != 6 || substs[0].path[0] != 4 {
		t.Errorf("the condition's call is written as its value; an arm's effect and effects outside the statement's span do not block: %+v %v", substs, ok)
	}
	bind := EmitEvent{kind: evDynBind, seq: 7, dyn: &emitDynBind{pos: gpos(11)}}
	if _, ok := NewEmitState().guardReruns(guardTree(bind), 10, body, 2); ok {
		t.Error("a bind inside the substituted paren is one the island would miss")
	}
	effect := EmitEvent{kind: evCall, seq: 7, call: emitCall{word: "print", pos: gpos(5)}}
	if _, ok := NewEmitState().guardReruns(guardTree(effect), 10, body, 2); ok {
		t.Error("an effect in the statement before the guard would run twice")
	}
}

func TestGuardPointAndPlan(t *testing.T) {
	body := guardBody()
	tree := guardTree()
	es := NewEmitState()
	rec := &fnUnitRec{frag: &EmitFragment{}, body: body}
	g := guardCand{seq: 10, kind: guardCond, op: EmitOperand{kind: opEvent, idx: 6}}
	d, ok := es.guardPoint(es.units[0], rec, tree, g)
	if !ok || d.token != 2 || d.guard != guardCond || !d.restart || len(d.substs) != 1 || d.substs[0].path[0] != 4 {
		t.Errorf("the condition's statement island, its paren substituted: %+v %v", d, ok)
	}
	effect := guardTree(EmitEvent{kind: evCall, seq: 7, call: emitCall{word: "print", pos: gpos(5)}})
	if _, ok := es.guardPoint(es.units[0], rec, effect, g); ok {
		t.Error("an effect before the guard takes no island")
	}
	tree[10].ev.br.condCheckPos = core.SrcPos{}
	if _, ok := es.guardPoint(es.units[0], rec, tree, g); ok {
		t.Error("an `if` with no position has no statement")
	}
	tree[10].ev.br.condCheckPos = gpos(7)
	noPos := append([]core.Value{core.NewWord("w")}, body[3:]...)
	if _, ok := es.guardPoint(es.units[0], &fnUnitRec{frag: &EmitFragment{}, body: noPos}, tree, g); ok {
		t.Error("a statement whose first token has no position cannot be restarted")
	}

	// The root's plan: nothing without a body, a planned island, and a
	// guard whose residual prefix cannot be placed.
	lw := &lowerer{es: es}
	es.planGuardRestarts(lw, nil)
	if lw.guardRestarts != nil {
		t.Error("no program body, no plan")
	}
	es.rootBody = body
	es.frames[0] = []EmitEvent{*tree[1].ev, *tree[6].ev, *tree[10].ev, *tree[12].ev}
	es.planGuardRestarts(lw, nil)
	if r := lw.guardRestarts[guardKey(10, guardCond)]; r == nil || r.token != 2 || r.depth != -1 || r.held != 0 || len(r.substs) != 1 {
		t.Errorf("the root plans the condition's island: %+v", r)
	}
	lw = &lowerer{es: es}
	es.planGuardRestarts(lw, []core.Value{core.NewInteger(3)})
	if lw.guardRestarts != nil {
		t.Error("a leading residual literal with no position is no placeable prefix")
	}
	es.frames[0] = []EmitEvent{*tree[1].ev, {kind: evCall, seq: 6, call: emitCall{word: "print", pos: gpos(11)}}, *tree[10].ev}
	es.frames[0][2].br = &emitBranch{condGuard: true, cond: EmitOperand{kind: opLocal}, pos: gpos(11), condCheckPos: gpos(7)}
	es.planGuardRestarts(lw, nil)
	if lw.guardRestarts != nil {
		t.Error("an effect before the guard in its statement plans no island")
	}
}

func TestGuardSeatAndEmit(t *testing.T) {
	flw := &lowerer{}
	seatDeoptPoint(flw, &fnUnitRec{}, deoptPoint{seq: 3, restart: true, guard: guardThen, substs: []substPlan{{path: []int{1}, seq: 2}}, token: 2, start: gpos(5)})
	r := flw.guardRestarts[guardKey(3, guardThen)]
	if r == nil || r.depth != -1 || r.held != -1 || len(r.substs) != 1 || len(flw.landingRestarts) != 0 {
		t.Fatalf("a unit's guard point is seated as a guard island: %+v", r)
	}
	flw.vm = []vmSlot{{seq: -1}}
	flw.noteRestartDepths(gpos(6))
	if !r.seated() || r.depth != 1 {
		t.Errorf("the walk seats a guard island's depth: %+v", r)
	}

	body := guardBody()
	var code []Instr
	var debug []core.SrcPos
	sig := &core.Signature{}
	lw := &lowerer{es: NewEmitState(), p: &Program{}, code: &code, debug: &debug, sigIdx: map[*core.Signature]int{}, landingBody: body, landingRoot: true, curBranch: 10,
		promoted: map[int]int{8: 3}, variadic: map[int]bool{}}
	lw.vm = []vmSlot{{seq: 5}, {seq: 6}}
	lw.guardRestarts = map[int]*landingRestart{guardKey(10, guardCond): {token: 2, depth: 0, held: 0, substs: []substPlan{
		{path: []int{4}, seq: 6}, {path: []int{6, 0}, seq: 5}, {path: []int{5}, seq: 8},
	}}}
	lw.emitGuardCallAt("__condguard", sig, gpos(7), guardCond, EmitOperand{kind: opEvent, idx: 6})
	if len(lw.p.Sigs) != 1 || len(lw.restartSigs) != 1 || len(code) != 1 || code[0].Op != OpCallNative {
		t.Fatalf("a seated guard calls a SigRef of its own: %+v %v", lw.p.Sigs, code)
	}
	is := lw.p.Sigs[0].Restart
	if is == nil || len(is.Island) != len(body)-2 || is.RetPC != -1 || !is.Root || len(is.Substs) != 3 ||
		is.Substs[0].Src.Kind != RestartGuard || is.Substs[0].Path[0] != 2 ||
		is.Substs[1].Src.Kind != RestartStack || is.Substs[1].Src.Idx != 0 || is.Substs[1].Path[0] != 4 || is.Substs[1].Path[1] != 0 ||
		is.Substs[2].Src.Kind != RestartLocal || is.Substs[2].Src.Idx != 3 {
		t.Errorf("the island from the statement on, each paren's value where the stop holds it: %+v", is)
	}
	// A value the stop no longer holds, or holds above a run-counted region,
	// seats no island: the guard keeps its designed defer.
	lw.guardRestarts[guardKey(10, guardElse)] = &landingRestart{token: 2, depth: 0, held: 0, substs: []substPlan{{path: []int{4}, seq: 9}}}
	lw.emitGuardCallAt("__codeguard", sig, gpos(7), guardElse, EmitOperand{kind: opLocal})
	lw.variadic[5] = true
	lw.guardRestarts[guardKey(10, guardThen)] = &landingRestart{token: 2, depth: 0, held: 0, substs: []substPlan{{path: []int{4}, seq: 6}}}
	lw.emitGuardCallAt("__codeguard", sig, gpos(7), guardThen, EmitOperand{kind: opLocal})
	if len(lw.p.Sigs) != 2 || lw.p.Sigs[1].Restart != nil || len(code) != 3 {
		t.Errorf("an unplaceable island falls back to the plain guard, one SigRef shared: %+v", lw.p.Sigs)
	}
	lw.emitGuardCallAt("__codeguard", nil, gpos(7), guardThen, EmitOperand{})
	if len(code) != 3 {
		t.Error("no guard, no call")
	}
	if _, ok := lw.restartSubstSrcs(nil, EmitOperand{}, -1); ok {
		t.Error("no island, no substitutions")
	}
	// The arm guard over the run's own stack: the walk's other operands are
	// hidden for the guard and restored after it.
	lw.vm = []vmSlot{{seq: 1}, {seq: 2}, {seq: 3}}
	br := &emitBranch{guard: sig, thenGuard: true}
	lw.emitCodeGuardOver(br, true, 1, vmSlot{seq: 3})
	if len(lw.vm) != 3 || lw.vm[2].seq != 3 || len(code) != 4 {
		t.Errorf("the view is restored after the guard: %+v", lw.vm)
	}
	if g := lw.armGuard(br, true); g.sig != sig || g.kind != guardThen {
		t.Errorf("the then arm's guard: %+v", g)
	}
	if g := lw.armGuard(br, false); g.sig != nil || g.kind != 0 {
		t.Errorf("an unguarded else arm: %+v", g)
	}
	br.elsGuard = true
	if g := lw.armGuard(br, false); g.sig != sig || g.kind != guardElse {
		t.Errorf("the else arm's guard: %+v", g)
	}
}

// TestFirstIterGuard pins the loops' first-iteration check a statement island
// takes at run time (NUR296): each enclosing counted loop's index slot and
// its constant start; a condition loop, or a start that is no constant
// integer, cannot be checked. A paren the island substitutes inside a loop
// would carry one iteration's value into every other.
func TestFirstIterGuard(t *testing.T) {
	es := NewEmitState()
	es.consts = []core.Value{core.NewInteger(0), core.NewString("s")}
	counted := &emitLoop{iterSlot: 4, start: EmitOperand{kind: opConst, idx: 0}}
	first, ok := es.firstIterGuard([]*emitLoop{counted})
	if !ok || len(first) != 1 || first[0].Slot != 4 || first[0].Val != 0 {
		t.Errorf("a counted loop's check is its slot and start: %+v %v", first, ok)
	}
	for _, c := range []struct {
		name string
		lp   *emitLoop
	}{
		{"a condition loop", &emitLoop{cond: &EmitFragment{}, start: EmitOperand{kind: opConst, idx: 0}}},
		{"a computed start", &emitLoop{start: EmitOperand{kind: opLocal, idx: 0}}},
		{"a start past the constants", &emitLoop{start: EmitOperand{kind: opConst, idx: 9}}},
		{"a start that is no integer", &emitLoop{start: EmitOperand{kind: opConst, idx: 1}}},
	} {
		if _, ok := es.firstIterGuard([]*emitLoop{counted, c.lp}); ok {
			t.Errorf("%s: no first-iteration check", c.name)
		}
	}
	tree := map[int]treeEvent{1: {inLoop: true}, 2: {}}
	if loopFree(tree, []substPlan{{seq: 2}, {seq: 1}}) || !loopFree(tree, []substPlan{{seq: 2}}) {
		t.Error("loopFree")
	}
}

// TestRestartRerunsInLoop pins a loop's statement island (NUR296): over
// counted loops it takes the first-iteration check, and an event written
// after the stop — which the pass may have recorded before it — is not the
// first iteration's run; over a condition loop the stop must be
// loop-invariant (restartSpanReruns).
func TestRestartRerunsInLoop(t *testing.T) {
	es := NewEmitState()
	es.consts = []core.Value{core.NewInteger(0)}
	read := EmitEvent{kind: evCall, seq: 9, call: emitCall{word: "dot", ops: []EmitOperand{{kind: opLocal, idx: 5}}, pos: gpos(10)}}
	bind := EmitEvent{kind: evDynBind, seq: 8, dyn: &emitDynBind{pos: gpos(20)}}
	loop := EmitEvent{kind: evLoop, seq: 12, loop: &emitLoop{iterSlot: 5, start: EmitOperand{kind: opConst, idx: 0}, pos: gpos(1),
		body: &EmitFragment{events: []EmitEvent{bind, read}}}}
	tree := rootTreeEvents([]EmitEvent{loop}, false)
	body := []core.Value{gtok(core.NewWord("for"), 1), gtok(core.NewEvalList([]core.Value{gtok(core.NewWord("l"), 10), gtok(core.NewWord("q"), 20)}), 8)}
	substs, first, ok := es.restartReruns(tree, tree[9], 9, body, 0)
	if !ok || len(substs) != 0 || len(first) != 1 || first[0].Slot != 5 {
		t.Errorf("a counted loop's stop restarts on its first iteration, the bind written after it aside: %+v %+v %v", substs, first, ok)
	}
	tree[12].ev.loop.cond = &EmitFragment{}
	if _, first, ok := es.restartReruns(tree, tree[9], 9, body, 0); ok || first != nil {
		t.Error("a condition loop's stop over the loop's index is no invariant read")
	}
}

// TestGuardPointInLoop pins a branch guard's island inside loops (NUR296):
// over a counted loop it takes the first-iteration check; a condition loop,
// or a paren substituted inside the loop, takes none.
func TestGuardPointInLoop(t *testing.T) {
	es := NewEmitState()
	es.consts = []core.Value{core.NewInteger(0)}
	body := guardBody()
	br := EmitEvent{kind: evBranch, seq: 10, br: &emitBranch{condGuard: true, cond: EmitOperand{kind: opLocal}, pos: gpos(11), condCheckPos: gpos(7)}}
	mk := func(cond *EmitFragment, extra ...EmitEvent) map[int]treeEvent {
		lp := EmitEvent{kind: evLoop, seq: 12, loop: &emitLoop{iterSlot: 3, start: EmitOperand{kind: opConst, idx: 0}, cond: cond, pos: gpos(5),
			body: &EmitFragment{events: append(extra, br)}}}
		return rootTreeEvents([]EmitEvent{lp}, false)
	}
	rec := &fnUnitRec{frag: &EmitFragment{}, body: body}
	g := guardCand{seq: 10, kind: guardCond, op: EmitOperand{kind: opLocal}}
	d, ok := es.guardPoint(es.units[0], rec, mk(nil), g)
	if !ok || len(d.first) != 1 || d.first[0].Slot != 3 {
		t.Errorf("a guard in a counted loop takes the first-iteration check: %+v %v", d, ok)
	}
	if _, ok := es.guardPoint(es.units[0], rec, mk(&EmitFragment{}), g); ok {
		t.Error("a guard in a condition loop takes no island")
	}
	call := EmitEvent{kind: evCallUser, seq: 6, uc: emitUserCall{pos: gpos(11), nout: 1}}
	if _, ok := es.guardPoint(es.units[0], rec, mk(nil, call), guardCand{seq: 10, kind: guardCond, op: EmitOperand{kind: opEvent, idx: 6}}); ok {
		t.Error("a paren substituted inside the loop would carry one iteration's value into every other")
	}
}

// doProgram is `(5 do [(mk)])` at column 1: the paren (token 0) holds 5,
// the do word (column 5) and the body list (column 8) whose one token is
// the paren (column 9) around mk (column 10).
func doProgram(bodyToks ...core.Value) []core.Value {
	if len(bodyToks) == 0 {
		bodyToks = []core.Value{gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 10)}), 9)}
	}
	return []core.Value{gtok(core.NewParenExpr([]core.Value{gtok(core.NewInteger(5), 2), gtok(core.NewWord("do"), 5),
		gtok(core.NewEvalList(bodyToks), 8)}), 1)}
}

// doState is an EmitState whose unit 0 is the do body's closure over events.
func doState(events ...EmitEvent) (*EmitState, *EmitEvent) {
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{{frag: &EmitFragment{events: events}}}
	return es, &EmitEvent{kind: evCall, seq: 4, call: emitCall{word: "do", nout: 1, pos: gpos(5), ops: []EmitOperand{{kind: opClosure, closureUnit: 0}}}}
}

// TestDoBody pins the statement island's `do` over a literal body (NUR286):
// a body of reads runs again whole, a body of one call — its paren or its
// bare word — has the do's own result written over the do word and its body
// list (the do word's path; the interpreter steps a do's result in the do's
// place), and any other do, body or event is no island's.
func TestDoBody(t *testing.T) {
	mkCall := EmitEvent{kind: evCallUser, seq: 3, uc: emitUserCall{pos: gpos(10), nout: 1}}
	es, do := doState(mkCall)
	sub, whole, ok := es.doBody(do, doProgram(), 0)
	if !ok || whole || len(sub) != 2 || sub[0] != 0 || sub[1] != 1 {
		t.Errorf("a body of one call has the do's result written over the do: %v %v %v", sub, whole, ok)
	}
	reads, rdo := doState(EmitEvent{kind: evCall, seq: 3, call: emitCall{word: "dot", pos: gpos(10)}})
	if _, whole, ok := reads.doBody(rdo, doProgram(), 0); !ok || !whole {
		t.Error("a body of reads runs again whole")
	}
	bare, bdo := doState(EmitEvent{kind: evCallUser, seq: 3, uc: emitUserCall{pos: gpos(9), nout: 1}})
	if sub, _, ok := bare.doBody(bdo, doProgram(gtok(core.NewWord("mk"), 9)), 0); !ok || len(sub) != 2 || sub[1] != 1 {
		t.Errorf("a body of one bare call word is written the same: %v %v", sub, ok)
	}
	two, tdo := doState(mkCall, EmitEvent{kind: evCallUser, seq: 2, uc: emitUserCall{pos: gpos(12), nout: 1}})
	if _, _, ok := two.doBody(tdo, doProgram(), 0); ok {
		t.Error("a body of two calls is no island's")
	}
	wide := doProgram(gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 10)}), 9), gtok(core.NewInteger(1), 14))
	if _, _, ok := es.doBody(do, wide, 0); ok {
		t.Error("a body with more tokens than the call's leaves another value")
	}
	notDo := doProgram()
	inner, _ := core.AsParenExpr(notDo[0])
	inner[1] = gtok(core.NewWord("each"), 6)
	if _, _, ok := es.doBody(do, notDo, 0); ok {
		t.Error("a body list another word takes is not the do's")
	}
	for _, c := range []struct {
		name string
		ev   *EmitEvent
	}{
		{"another word", &EmitEvent{kind: evCall, call: emitCall{word: "each", nout: 1, ops: do.call.ops}}},
		{"a do over no closure", &EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 1, ops: []EmitOperand{{kind: opLocal}}}}},
		{"a do of two results", &EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 2, ops: do.call.ops}}},
		{"a unit past the table", &EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 1, ops: []EmitOperand{{kind: opClosure, closureUnit: 5}}}}},
	} {
		if _, _, ok := es.doBody(c.ev, doProgram(), 0); ok {
			t.Errorf("%s: no island's", c.name)
		}
	}
	es.fnRecs[0].lambdaUnit = true
	if _, _, ok := es.doBody(do, doProgram(), 0); ok {
		t.Error("a lambda's unit escapes the frame")
	}
	es.fnRecs[0].lambdaUnit = false
	// The plan: the do and its body list, two tokens, are written as the
	// do's result, the do's own event and its body's paren covered by it; a
	// whole body's do runs again.
	tree := map[int]treeEvent{4: {ev: do}, 3: {ev: &mkCall}}
	got, ok := es.restartSubsts(tree, doProgram(), 0, []int{3, 4})
	if !ok || len(got) != 1 || got[0].seq != 4 || len(got[0].path) != 2 || got[0].span != 2 {
		t.Errorf("the do's result stands for the do and its body: %+v %v", got, ok)
	}
	if got, ok := reads.restartSubsts(map[int]treeEvent{4: {ev: rdo}}, doProgram(), 0, []int{4}); !ok || len(got) != 0 {
		t.Errorf("a whole body's do runs again: %+v %v", got, ok)
	}
}

// TestWordlessRestartAndLandedSource pins a landing with no word after it
// (NUR286): its entry carries only the statement island of a landing over
// its own values beneath, and the landed value's source is the stack's top
// entry — just above the walk's stack for a call, whose seating comes after
// the landing, or the walk's top slot for a branch's merge.
func TestWordlessRestartAndLandedSource(t *testing.T) {
	es := NewEmitState()
	es.landingOwn = map[int]landingStep{4: {beneath: true}, 5: {}}
	var code []Instr
	words := map[int]LandingWord(nil)
	lw := &lowerer{es: es, code: &code, landingWords: &words, landingBody: doProgram(), landingRoot: true, promoted: map[int]int{4: 7}}
	lw.vm = []vmSlot{{seq: -1}}
	lw.landingRestarts = map[int]*landingRestart{4: {token: 0, depth: 0, held: 0, substs: []substPlan{{path: []int{0, 1}, span: 2, seq: 4}}},
		5: {token: 0, depth: 0, held: 0}}
	lw.seatLandingWord(LandingWord{}, 5)
	lw.seatLandingWord(LandingWord{}, 6)
	if len(words) != 0 {
		t.Errorf("no island, or none over values beneath, seats nothing: %+v", words)
	}
	lw.seatLandingWord(LandingWord{}, 4)
	w, ok := words[0]
	if !ok || !w.Restart || len(w.Substs) != 1 || w.Substs[0].Src.Kind != RestartStack || w.Substs[0].Src.Idx != 1 || w.Substs[0].Span != 2 {
		t.Fatalf("the landed value is the entry just above the walk's stack, never its unwritten slot: %+v", words)
	}
	lw.vm = append(lw.vm, vmSlot{seq: 4})
	if got := lw.landedIdx(4); got != 1 {
		t.Errorf("a merge's landed value is the walk's top slot: %d", got)
	}
	lw.landingRestarts[4].substs[0].seq = 9
	lw.seatLandingWord(LandingWord{}, 4)
	if len(words) != 1 {
		t.Errorf("a value held nowhere seats no island: %+v", words)
	}
}

// TestSubstPlanCovers pins the run of tokens a substitution replaces (NUR286,
// NUR297): a paren's run is its own token and everything inside it, a do's
// is the do word and its body list; and the plans are ordered as their
// tokens stand, so the VM, writing them last to first, never moves a later
// one's path.
func TestSubstPlanCovers(t *testing.T) {
	paren := substPlan{path: []int{2, 1}, span: 1}
	do := substPlan{path: []int{0}, span: 2}
	for _, c := range []struct {
		name string
		p    substPlan
		path []int
		want bool
	}{
		{"a paren's own token", paren, []int{2, 1}, true},
		{"a token inside the paren", paren, []int{2, 1, 0}, true},
		{"the paren's neighbour", paren, []int{2, 2}, false},
		{"another enclosing token", paren, []int{3, 1}, false},
		{"the token enclosing the paren", paren, []int{2}, false},
		{"the do word", do, []int{0}, true},
		{"inside the do's body list", do, []int{1, 0}, true},
		{"the token after the body", do, []int{2}, false},
		{"no plan path", substPlan{span: 1}, []int{0}, false},
	} {
		if got := c.p.covers(c.path); got != c.want {
			t.Errorf("%s: covers = %v, want %v", c.name, got, c.want)
		}
	}
	for _, c := range []struct {
		a, b []int
		want bool
	}{
		{[]int{0}, []int{2, 1}, true},
		{[]int{2, 1}, []int{0}, false},
		{[]int{2}, []int{2, 1}, true},
		{[]int{2, 1}, []int{2}, false},
		{[]int{1, 3}, []int{1, 4}, true},
	} {
		if got := pathLess(c.a, c.b); got != c.want {
			t.Errorf("pathLess(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestWordLedSubstNeedsInertPredecessors pins the barrier rule (NUR296): a
// word-led run written as a value — a bare call, a do and its body — is a
// collection barrier no longer, so it is written only where every token
// before it on its level is a scalar literal. A paren is collectable either
// way and needs no such proof.
func TestWordLedSubstNeedsInertPredecessors(t *testing.T) {
	five := gtok(core.NewInteger(5), 1)
	y := gtok(core.NewWord("y"), 3)
	lead := gtok(core.NewWord("m"), 1)
	if !wordAt([]core.Value{five, y}, []int{1}) || wordAt([]core.Value{five, y}, []int{0}) {
		t.Error("wordAt names a bare word's token")
	}
	for _, c := range []struct {
		name string
		body []core.Value
		tok  int
		path []int
		want bool
	}{
		{"literals before it at the top", []core.Value{five, five, y}, 0, []int{2}, true},
		{"a word before it at the top", []core.Value{lead, y}, 0, []int{1}, false},
		{"before the statement's first token", []core.Value{lead, five, y}, 1, []int{2}, true},
		{"inside a paren, from its start", []core.Value{lead, gtok(core.NewParenExpr([]core.Value{five, y}), 2)}, 0, []int{1, 1}, true},
		{"inside a paren, after a word", []core.Value{gtok(core.NewParenExpr([]core.Value{lead, y}), 2)}, 0, []int{0, 1}, false},
	} {
		if got := inertBefore(c.body, c.tok, c.path); got != c.want {
			t.Errorf("%s: inertBefore = %v, want %v", c.name, got, c.want)
		}
	}
	// The planner: a bare call after a word is no candidate, so its event,
	// outside any written paren, blocks the island.
	body := []core.Value{lead, gtok(core.NewWord("y"), 3)}
	bare := map[int]treeEvent{1: {ev: &EmitEvent{kind: evCallUser, seq: 1, uc: emitUserCall{pos: gpos(3), nout: 1}}}}
	if _, ok := NewEmitState().restartSubsts(bare, body, 0, []int{1}); ok {
		t.Error("a bare call after a collector is not written as its value")
	}
	if got, ok := NewEmitState().restartSubsts(bare, []core.Value{five, gtok(core.NewWord("y"), 3)}, 0, []int{1}); !ok || len(got) != 1 || got[0].span != 1 {
		t.Errorf("after a literal it is: %+v %v", got, ok)
	}
}

// TestStashSubst pins the stash (NUR296): a planned island's substituted
// value its call left on the stack's top is copied into a slot of its own
// (STORE_LOCAL, PUSH_LOCAL), which the island reads where the statement
// consumed the value before the stop; any other event, a value not on the
// top, and a promoted one keep no stash.
func TestStashSubst(t *testing.T) {
	var code []Instr
	var dbg []core.SrcPos
	lw := &lowerer{es: NewEmitState(), code: &code, debug: &dbg, promoted: map[int]int{8: 2}}
	lw.guardRestarts = map[int]*landingRestart{guardKey(1, guardCond): {substs: []substPlan{{path: []int{0}, span: 1, seq: 5}, {path: []int{1}, span: 1, seq: 8}}}}
	lw.landingRestarts = map[int]*landingRestart{9: {substs: []substPlan{{path: []int{2}, span: 1, seq: 6}}}}
	if !lw.substSeq(5) || !lw.substSeq(6) || lw.substSeq(7) {
		t.Error("substSeq names each planned island's substituted events")
	}
	ev := func(seq int) *EmitEvent {
		return &EmitEvent{kind: evCallUser, seq: seq, uc: emitUserCall{pos: gpos(3), nout: 1}}
	}
	lw.vm = []vmSlot{{seq: 5}}
	lw.stashSubst(ev(5))
	if t5, ok := lw.substStash[5]; !ok || len(code) != 2 || code[0].Op != OpStoreLocal || code[1].Op != OpPushLocal || int(code[0].Arg) != t5 || len(lw.vm) != 1 {
		t.Fatalf("the value is stored and pushed back, its slot kept: %+v %v", code, lw.substStash)
	}
	lw.vm = []vmSlot{{seq: 7}}
	lw.stashSubst(ev(7))
	lw.vm = []vmSlot{{seq: 3}}
	lw.stashSubst(ev(6))
	lw.vm = []vmSlot{{seq: 8}}
	lw.stashSubst(ev(8))
	if len(code) != 2 || len(lw.substStash) != 1 {
		t.Errorf("no plan, not on the top, promoted: no stash: %+v", code)
	}
	// The island reads the stash where the value is held nowhere else.
	r := &landingRestart{token: 0, substs: []substPlan{{path: []int{0}, span: 1, seq: 5}}}
	lw.vm = nil
	got, ok := lw.restartSubstSrcs(r, EmitOperand{}, -1)
	if !ok || len(got) != 1 || got[0].Src.Kind != RestartLocal || got[0].Src.Idx != lw.substStash[5] {
		t.Errorf("a consumed value is read from its stash: %+v %v", got, ok)
	}
}
