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
	if len(got) != 3 || got[0].kind != guardCond || got[1].kind != guardThen || got[2].kind != guardElse || got[2].op.idx != 2 {
		t.Errorf("a branch outside any loop lists its guarded operands in order: %+v", got)
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
		{"a word, not a paren", mk(7, 1), nil, false},
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
	got, ok := restartSubsts(tree, body, 2, []int{3, 6, 7})
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
	if got, ok := restartSubsts(nested, deep, 0, []int{1, 2}); !ok || len(got) != 1 || got[0].seq != 1 {
		t.Errorf("the outermost paren is written: %+v %v", got, ok)
	}
	bind := map[int]treeEvent{1: {ev: &EmitEvent{kind: evCallUser, seq: 1, uc: emitUserCall{pos: gpos(11), nout: 1}}}, 2: {ev: &EmitEvent{kind: evDynBind, seq: 2, dyn: &emitDynBind{pos: gpos(11)}}}}
	if _, ok := restartSubsts(bind, body, 2, []int{1, 2}); ok {
		t.Error("a bind inside a written paren is one the island would miss")
	}
	effect := map[int]treeEvent{1: {ev: &EmitEvent{kind: evCall, seq: 1, call: emitCall{word: "print", pos: gpos(5)}}}}
	if _, ok := restartSubsts(effect, body, 2, []int{1}); ok {
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
	substs, ok := guardReruns(guardTree(), 10, body, 2)
	if !ok || len(substs) != 1 || substs[0].seq != 6 || substs[0].path[0] != 4 {
		t.Errorf("the condition's call is written as its value; an arm's effect and effects outside the statement's span do not block: %+v %v", substs, ok)
	}
	bind := EmitEvent{kind: evDynBind, seq: 7, dyn: &emitDynBind{pos: gpos(11)}}
	if _, ok := guardReruns(guardTree(bind), 10, body, 2); ok {
		t.Error("a bind inside the substituted paren is one the island would miss")
	}
	effect := EmitEvent{kind: evCall, seq: 7, call: emitCall{word: "print", pos: gpos(5)}}
	if _, ok := guardReruns(guardTree(effect), 10, body, 2); ok {
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
	if _, ok := lw.restartSubstSrcs(nil, EmitOperand{}); ok {
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
