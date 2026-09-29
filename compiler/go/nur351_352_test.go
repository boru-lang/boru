package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur351_352_test.go dials the NUR351 / NUR352 / NUR343 planning helpers
// directly. The whole-program pins are lang's nur351_352_test.go and
// nur343_349_test.go.

// nurRegistry is a registry naming keys-like word `k` (one forward slot)
// and a stack-only word `s`, and binding the value `v`.
func nurRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r := newTestRegistry(t)
	r.Register("k", core.Signature{Args: []*core.Type{core.TMap}, BarrierPos: -1})
	r.Register("s", core.Signature{Args: []*core.Type{core.TAny}, BarrierPos: 0})
	r.Defs.Push("v", core.NewInteger(3))
	return r
}

// collectorToken names the word written before a bare read whose forward
// window may reach it: a call of a registered word within its reach, with no
// function word between, or a word the registry does not name.
func TestCollectorTokenArms(t *testing.T) {
	es := NewEmitState()
	es.reg = nurRegistry(t)
	ref := deoptTok("k", 3)
	w, _ := core.AsWord(ref)
	w.ForceVal = true
	refTok := core.NewWord("k")
	refTok.Data = w
	refTok.SetPos(deoptAt(3))
	reach := deoptLit(core.NewList([]core.Value{deoptTok("x", 12)}), 11)
	body := []core.Value{deoptTok("k", 1), deoptTok("x", 3), deoptTok("u", 5), deoptTok("s", 7), deoptTok("x", 9), reach, deoptLit(core.NewInteger(1), 13)}
	call := func(col int) *EmitEvent {
		return &EmitEvent{kind: evCall, call: emitCall{word: "w", pos: deoptAt(col)}}
	}
	for _, c := range []struct {
		name    string
		body    []core.Value
		ev      *EmitEvent
		readTok int
		want    int
	}{
		{"a word within its reach", body, call(1), 1, 0},
		{"a word beyond its reach", body, call(1), 4, -1},
		{"an unnamed word reaches, but a function word stands between", body, call(5), 4, -1},
		{"an unnamed word right before the read", body, call(5), 3, 2},
		{"a user call at its word", body, &EmitEvent{kind: evCallUser, uc: emitUserCall{wordPos: deoptAt(5), pos: deoptAt(9)}}, 3, 2},
		{"a live read is no dispatch", body, &EmitEvent{kind: evCall, call: emitCall{live: true, pos: deoptAt(1)}}, 1, -1},
		{"another kind", body, &EmitEvent{kind: evStore, store: &emitStore{pos: deoptAt(1)}}, 1, -1},
		{"the word after the read", body, call(5), 1, -1},
		{"a literal is no word", body, call(13), 5, -1},
		{"a read inside a list collects whatever it holds", body, call(1), 5, -1},
		{"a /v spelling is data", []core.Value{refTok, deoptTok("x", 5)}, call(3), 1, -1},
	} {
		if got := es.collectorToken(c.body, c.ev, c.readTok); got != c.want {
			t.Errorf("%s: collectorToken = %d, want %d", c.name, got, c.want)
		}
	}
	// No registry bound: every word may reach.
	es.reg = nil
	if got := es.collectorToken(body, call(1), 4); got != 0 {
		t.Errorf("an unbound pass takes a word to reach: %d", got)
	}
	if es.lookupWord("k") != nil {
		t.Error("no registry names no word")
	}
}

// collectorBeforeRead: a statement word before a bare read, the read's own
// consumer aside, owes the read a point.
func TestCollectorBeforeRead(t *testing.T) {
	es := NewEmitState()
	body := []core.Value{deoptTok("k", 1), deoptTok("x", 3)}
	rec := &fnUnitRec{body: body, frag: &EmitFragment{events: []EmitEvent{{kind: evCall, call: emitCall{word: "k", pos: deoptAt(1)}}}}}
	if !es.collectorBeforeRead(rec, deoptAt(3), 0, -1) {
		t.Error("a word before the read owes it a point")
	}
	if es.collectorBeforeRead(rec, deoptAt(3), 0, 0) {
		t.Error("the read's own consumer is served at its push")
	}
	if es.collectorBeforeRead(rec, deoptAt(3), 1, -1) {
		t.Error("a word of an earlier statement is none")
	}
	if es.collectorBeforeRead(rec, deoptAt(4), 0, -1) {
		t.Error("a read at no token has none")
	}
}

// keptRunBeforeRead: a computed body run between the test and the read —
// a dyn-body dispatch, or a call of a unit that runs one — would be read
// before it ran.
func TestKeptRunBeforeRead(t *testing.T) {
	es := NewEmitState()
	es.eventInfo = map[int]eventFlags{2: {dynBodyResult: true}}
	es.fnRecs = []*fnUnitRec{{runsKeptDefs: "do"}, {}}
	read := EmitEvent{seq: 5, kind: evCall, call: emitCall{live: true}}
	do := EmitEvent{seq: 2, kind: evCall, call: emitCall{word: "do"}}
	runner := EmitEvent{seq: 3, kind: evCallUser, uc: emitUserCall{unit: 0}}
	plain := EmitEvent{seq: 4, kind: evCallUser, uc: emitUserCall{unit: 1}}
	if !es.keptRunBeforeRead([]EmitEvent{do, read}, 0, 5) || !es.keptRunBeforeRead([]EmitEvent{runner, read}, 0, 5) {
		t.Error("a computed body run after the test and before the read declines")
	}
	if es.keptRunBeforeRead([]EmitEvent{do, read}, 1, 5) || es.keptRunBeforeRead([]EmitEvent{plain, read}, 0, 5) {
		t.Error("a run before the test, or a call that runs none, is no bar")
	}
}

// liveReadUnserved: a read owed a point that none serves declines; a served
// one, or one owed none, lowers.
func TestLiveReadUnserved(t *testing.T) {
	es := NewEmitState()
	es.liveNeedsPoint = map[int]bool{5: true}
	lw := &lowerer{es: es}
	if r := lw.liveReadUnserved(5); !strings.Contains(r, "NUR351") {
		t.Errorf("an unserved read declines: %q", r)
	}
	lw.liveServed = map[int]bool{5: true}
	if r := lw.liveReadUnserved(5); r != "" {
		t.Errorf("a served read lowers: %q", r)
	}
	if r := (&lowerer{es: es}).liveReadUnserved(6); r != "" {
		t.Errorf("a read owed no point lowers: %q", r)
	}
	if r := (&lowerer{}).liveReadUnserved(5); r != "" {
		t.Errorf("no recorder, no decline: %q", r)
	}
}

// trapBeforeStaleRead: after a root computed body, a trap with a data word
// or an unbound word after it in its statement is no proof; a fn word, a
// type name, a reserved literal or the statement's end are.
func TestTrapBeforeStaleRead(t *testing.T) {
	es := NewEmitState()
	es.reg = nurRegistry(t)
	es.rootBody = []core.Value{
		deoptLit(core.NewInteger(4), 1), deoptTok("k", 3), deoptLit(core.NewInteger(2), 5), deoptTok("s", 7), deoptTok("Integer", 9), deoptTok("true", 11), deoptTok("end", 13),
		deoptTok("k", 15), deoptTok("v", 17), deoptTok("end", 19), deoptTok("k", 21), deoptTok("zz", 23),
		deoptLit(core.NewParenExpr([]core.Value{deoptTok("k", 27), deoptTok("v", 29)}), 25),
		deoptLit(core.NewParenExpr([]core.Value{deoptTok("k", 33)}), 31),
	}
	es.rootBody[6] = deoptLit(core.NewEnd(), 13)
	es.rootBody[9] = deoptLit(core.NewEnd(), 19)
	if es.trapBeforeStaleRead(deoptAt(3)) {
		t.Error("no computed body ran: the trap stands")
	}
	es.rootDynLeak = true
	for _, c := range []struct {
		name string
		at   int
		want bool
	}{
		{"a literal, a fn word, a type name and a literal word, to the end", 3, false},
		{"a data word", 15, true},
		{"an unbound word", 21, true},
		{"a trap at no token", 4, false},
		{"a trap inside a paren, a data word after it there", 27, true},
		{"a trap inside a paren, nothing after it there", 33, false},
	} {
		if got := es.trapBeforeStaleRead(deoptAt(c.at)); got != c.want {
			t.Errorf("%s: trapBeforeStaleRead = %v, want %v", c.name, got, c.want)
		}
	}
	es.reg = nil
	if es.trapBeforeStaleRead(deoptAt(15)) {
		t.Error("no registry bound: no read to tell")
	}
}

// rootStackHeld is the root events the stack told at a statement's first
// token holds on the compiled stack — nil when the stack was not told or
// holds a value no event left there.
func TestRootStackHeld(t *testing.T) {
	es := NewEmitState()
	end := deoptLit(core.NewEnd(), 3)
	es.rootBody = []core.Value{deoptTok("do", 1), end, deoptTok("k", 5)}
	lw := &lowerer{es: es, promoted: map[int]int{}}
	if es.rootStackHeld(lw, 2) != nil {
		t.Error("an untold stack holds nothing")
	}
	run := core.NewDynamicCarrier(core.TAny)
	es.producedBy[run.ID] = producer{seq: 7}
	es.rootStmtStacks = map[core.SrcPos][]core.Value{end.Pos(): {run}}
	if held := es.rootStackHeld(lw, 2); !held[7] || len(held) != 1 {
		t.Errorf("the body's run is held beneath the statement: %v", held)
	}
	es.rootStmtStacks[end.Pos()] = []core.Value{core.NewCarrier(core.TInteger)}
	if es.rootStackHeld(lw, 2) != nil {
		t.Error("a carrier no event left cannot be seated")
	}
	es.rootStmtStacks[end.Pos()] = []core.Value{core.NewInteger(4)}
	if es.rootStackHeld(lw, 2) != nil {
		t.Error("a constant is no compiled-stack entry")
	}
}

// spelledBefore finds the statement's token spelling a positionless literal.
func TestSpelledBefore(t *testing.T) {
	mapOf := func(n int64) core.Value {
		om := core.NewOrderedMap()
		om.Set("b", core.NewInteger(n))
		return core.NewMap(om)
	}
	body := []core.Value{deoptTok("k", 1), deoptLit(mapOf(1), 3), deoptTok("x", 5)}
	if got := spelledBefore(body, mapOf(1), 0, 2); got != 1 {
		t.Errorf("the map token spells it: %d", got)
	}
	if got := spelledBefore(body, mapOf(2), 0, 3); got != -1 {
		t.Errorf("no token spells another map: %d", got)
	}
}

// The operand rules of pendingStart: a live read lowered before the test
// whose value a later call takes, and a value of an earlier statement a
// later call takes.
func TestPendingOperandRules(t *testing.T) {
	body := []core.Value{deoptTok("x", 1), deoptTok("add", 3), deoptTok("y", 5)}
	x := EmitEvent{seq: 1, kind: evCall, call: emitCall{live: true, pos: deoptAt(1)}}
	prior := EmitEvent{seq: 0, kind: evCall, call: emitCall{word: "do", pos: core.SrcPos{Row: 0, Col: 0}}}
	add := EmitEvent{seq: 2, kind: evCall, call: emitCall{word: "add", pos: deoptAt(3), ops: []EmitOperand{EventOperand(1, 0)}}}
	var moved []core.SrcPos
	back := func(p core.SrcPos, _ int) bool {
		moved = append(moved, p)
		return true
	}
	if !liveOperandBefore([]EmitEvent{x, add}, body, 1, back) || len(moved) != 1 || moved[0] != deoptAt(1) {
		t.Errorf("the live read the call takes moves the start to it: %v", moved)
	}
	if liveOperandBefore([]EmitEvent{prior, add}, body, 1, back) {
		t.Error("an operand of no live read moves nothing")
	}
	stmt := deoptAt(3)
	early := EmitEvent{seq: 1, kind: evCall, call: emitCall{word: "do", pos: deoptAt(1)}}
	if !priorOperandAfter([]EmitEvent{early, add}, 1, stmt) {
		t.Error("a value of an earlier statement the call takes is the told stack's")
	}
	if priorOperandAfter([]EmitEvent{early, add}, 1, deoptAt(1)) || priorOperandAfter([]EmitEvent{prior, add}, 1, stmt) {
		t.Error("a value of the statement itself, or of no placed event, is none")
	}
}

// pendingListAt finds the pending list literal written at p, at any depth.
func TestPendingListAt(t *testing.T) {
	inner := deoptLit(core.NewList([]core.Value{deoptLit(core.NewInteger(1), 8)}), 7)
	inner.Eval = true
	outer := deoptLit(core.NewList([]core.Value{deoptTok("x", 5), inner}), 4)
	outer.Eval = true
	quoted := deoptLit(core.NewList([]core.Value{deoptLit(core.NewInteger(2), 12)}), 11)
	quoted.Eval, quoted.Quoted = true, true
	body := []core.Value{deoptTok("each", 1), outer, quoted}
	if v, ok := pendingListAt(body, deoptAt(7)); !ok || v.Pos() != deoptAt(7) {
		t.Errorf("the nested literal: %v %v", v, ok)
	}
	if _, ok := pendingListAt(body, deoptAt(11)); ok {
		t.Error("a quoted list is no pending literal")
	}
	if _, ok := pendingListAt(body, deoptAt(1)); ok {
		t.Error("a word is no list")
	}
	if _, ok := pendingListAt(body, core.SrcPos{}); ok {
		t.Error("no position, no literal")
	}
	if _, ok := pendingListAt(body, deoptAt(0)); ok {
		t.Error("no token there")
	}
}

// polyRawOperands: the poly's operands assembled from a pending literal of
// the body, by window index; RenderWindow puts them back for the report.
func TestPolyRawOperands(t *testing.T) {
	lit := deoptLit(core.NewList([]core.Value{deoptLit(core.NewInteger(1), 7), deoptTok("add", 9)}), 6)
	lit.Eval = true
	body := []core.Value{deoptTok("each", 1), deoptTok("h", 3), lit}
	events := []EmitEvent{
		{seq: 1, kind: evCallUser},
		{seq: 2, kind: evCall, call: emitCall{makeList: true, pos: deoptAt(6)}},
		{seq: 3, kind: evCall, call: emitCall{makeList: true, pos: deoptAt(3)}},
	}
	lw := &lowerer{landingBody: body, scopes: [][]EmitEvent{events}, vm: []vmSlot{{seq: 3}, {seq: 1}, {seq: 2}, {seq: 9}}}
	raw := lw.polyRawOperands(3)
	if len(raw) != 1 || raw[1].Pos() != deoptAt(6) {
		t.Fatalf("the literal's window index: %v", raw)
	}
	lw.vm = []vmSlot{{seq: 2, idx: 1}}
	if raw := lw.polyRawOperands(1); raw != nil {
		t.Errorf("another result of the event is none: %v", raw)
	}
	unit := &lowerer{rec: &fnUnitRec{body: body}}
	if len(unit.sourceBody()) != 3 || len(lw.sourceBody()) != 3 || (&lowerer{}).sourceBody() != nil {
		t.Error("the source body is the unit's where no landing body is seated")
	}
	pr := &PolyRef{}
	win := []core.Value{core.NewInteger(3), core.NewList([]core.Value{core.NewInteger(3)})}
	if got := pr.RenderWindow(win); &got[0] != &win[0] {
		t.Error("no raw operand renders the window itself")
	}
	pr.Raw = map[int]core.Value{1: lit, 5: lit}
	got := pr.RenderWindow(win)
	if got[1].Pos() != deoptAt(6) || got[0].String() != "3" || win[1].Pos() == deoptAt(6) {
		t.Errorf("the literal renders in its slot, the window unchanged: %v %v", got, win)
	}
}

// unrerunBranchBefore: a rematch after a branch the island cannot run again
// declines; one after a re-runnable branch, or no branch, keeps its defer.
func TestUnrerunBranchBefore(t *testing.T) {
	effect := &EmitFragment{events: []EmitEvent{{seq: 4, kind: evCall, call: emitCall{word: "print"}}}}
	read := &EmitFragment{events: []EmitEvent{{seq: 5, kind: evCall, call: emitCall{word: "dot"}}}}
	tree := map[int]treeEvent{
		1: {ev: &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{then: effect}}},
		2: {ev: &EmitEvent{seq: 2, kind: evBranch, br: &emitBranch{then: read}}},
		3: {ev: &EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "print"}}},
	}
	if r := unrerunBranchBefore(tree, []int{2, 1}); !strings.Contains(r, "NUR343") {
		t.Errorf("an effect arm declines: %q", r)
	}
	if r := unrerunBranchBefore(tree, []int{2, 3}); r != "" {
		t.Errorf("a re-runnable branch and a plain call keep the defer: %q", r)
	}
}

// recordArmTrap: a raise the model met evaluating an arm's pending literal
// is no arm trap: none is recorded, and the pass's failure stands.
func TestArmResidualRaiseDeclines(t *testing.T) {
	es := NewEmitState()
	r := newTestRegistry(t)
	defer es.BindRegistry(r)()
	es.ArmSealedBranchCapture()
	end := es.BodyAnalysisGuard()
	r.Check.ArmResidualSweep = 1
	if es.RecordArmTrapErr(&core.BoruError{Code: "signature_error"}, deoptAt(3)) {
		t.Error("the sweep's raise is not recorded")
	}
	r.Check.ArmResidualSweep = 0
	// Out of the sweep, the same raise in the sealed arm is its trap.
	if !es.RecordArmTrapErr(&core.BoruError{Code: "signature_error"}, deoptAt(3)) {
		t.Error("the arm's own raise is its trap")
	}
	end()
	es.TakeFragment()
}

// rootReadStatementPoint declines where the statement island cannot serve
// the read: a read made twice or held on the residual, a value in no slot,
// a read at no statement.
func TestRootReadStatementPointDeclines(t *testing.T) {
	es := NewEmitState()
	es.rootBody = []core.Value{deoptLit(core.NewInteger(1), 1), deoptTok("x", 3)}
	lw := &lowerer{es: es, promoted: map[int]int{4: 0}}
	rec := &fnUnitRec{frag: &EmitFragment{}, body: es.rootBody}
	one := rootWordRead{name: "x", reads: []core.SrcPos{deoptAt(3)}}
	for _, c := range []struct {
		name string
		r    rootWordRead
		seq  int
		also bool
	}{
		{"read twice", rootWordRead{name: "x", reads: []core.SrcPos{deoptAt(3), deoptAt(3)}}, 4, false},
		{"held on the residual too", one, 4, true},
		{"a value in no slot", one, 5, false},
		{"a read at no statement", rootWordRead{name: "x", reads: []core.SrcPos{deoptAt(0)}}, 4, false},
		{"a read with no consumer event", rootWordRead{name: "x", reads: []core.SrcPos{deoptAt(9)}}, 4, false},
	} {
		if _, ok := es.rootReadStatementPoint(lw, rec, c.r, c.seq, 0, c.also, nil); ok {
			t.Errorf("%s: no point", c.name)
		}
	}
}
