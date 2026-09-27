package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur296_callrun_test.go pins the call run (landing_restart.go, NUR296's
// effect form): where a call's arguments were written (noteArgSites), the
// run of tokens a bare call took (callRun), the tokens no forward collection
// passes through (barrierFree, writtenOver) and the effect written as no
// token (RestartNone). The lang suite drives it end to end
// (TestNUR296EffectBeforeTheStop).

func TestNoteArgSites(t *testing.T) {
	es := NewEmitState()
	es.readPos = map[string]core.SrcPos{"r": gpos(15)}
	es.producedBy["p"] = producer{seq: 4}
	val := func(id string, col int) core.Value {
		v := core.NewInteger(1)
		v.ID = id
		v.SetPos(gpos(col))
		return v
	}
	es.noteArgSites(9, []core.Value{val("lit", 7), val("r", 3), val("p", 2), val("", 5)})
	got := es.argSites[9]
	if len(got) != 4 || got[0] != (argSite{pos: gpos(7), seq: -1}) || got[1] != (argSite{pos: gpos(15), seq: -1}) ||
		got[2].seq != 4 || got[3] != (argSite{pos: gpos(5), seq: -1}) {
		t.Errorf("a literal by its token, a read by its site, a computed value by its event: %+v", got)
	}
}

// runBody is `print "a" 5 (g) w`: print at column 1, its argument at 7, a
// paren at 13 holding g at 14, a word at 20.
func runBody() []core.Value {
	return []core.Value{
		gtok(core.NewWord("print"), 1), gtok(core.NewString("a"), 7), gtok(core.NewInteger(5), 11),
		gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("g"), 14)}), 13), gtok(core.NewWord("w"), 20),
	}
}

func TestCallRun(t *testing.T) {
	es := NewEmitState()
	es.argSites = map[int][]argSite{}
	body := runBody()
	call := func(seq, col, nout int, sites ...argSite) *EmitEvent {
		ops := make([]EmitOperand, len(sites))
		es.argSites[seq] = sites
		return &EmitEvent{kind: evCall, seq: seq, call: emitCall{word: "print", pos: gpos(col), ops: ops, nout: nout}}
	}
	lit := func(col int) argSite { return argSite{pos: gpos(col), seq: -1} }
	g := &EmitEvent{kind: evCallUser, seq: 3, uc: emitUserCall{pos: gpos(14), nout: 1}}
	tree := map[int]treeEvent{3: {ev: g}}
	for _, c := range []struct {
		name string
		ev   *EmitEvent
		tok  int
		path []int
		span int
		ok   bool
	}{
		{"an effect over the literal after it", call(1, 1, 0, lit(7)), 0, []int{0}, 2, true},
		{"its value over two tokens, the paren's by its event", call(2, 1, 1, lit(7), lit(11), argSite{seq: 3}), 0, []int{0}, 4, true},
		{"two values of one token", call(4, 1, 1, lit(7), lit(7)), 0, []int{0}, 2, true},
		{"a word that took nothing", call(5, 20, 0), 0, []int{4}, 1, true},
		{"a run of several results", call(6, 1, 2, lit(7)), 0, nil, 0, false},
		{"an argument off the stack", call(7, 20, 0, lit(11)), 0, nil, 0, false},
		{"a gap in the run", call(8, 1, 0, lit(11)), 0, nil, 0, false},
		{"an argument at the word itself", call(9, 1, 0, lit(1)), 0, nil, 0, false},
		{"a computed argument no event of the tree left", call(10, 1, 0, argSite{seq: 99}), 0, nil, 0, false},
		{"a call before the statement", call(11, 1, 0, lit(7)), 1, nil, 0, false},
		{"a call at a value, not a word", call(12, 11, 0), 0, nil, 0, false},
		{"a call written nowhere", call(13, 40, 0), 0, nil, 0, false},
	} {
		path, span, ok := es.callRun(tree, c.ev, body, c.tok)
		if ok != c.ok || span != c.span || len(path) != len(c.path) || (ok && path[0] != c.path[0]) {
			t.Errorf("%s: callRun = %v %d %v, want %v %d %v", c.name, path, span, ok, c.path, c.span, c.ok)
		}
	}
	varied := call(23, 1, 1, lit(7))
	es.eventInfo[23] = eventFlags{variadicResult: true}
	if _, _, ok := es.callRun(tree, varied, body, 0); ok {
		t.Error("a call whose run's count varies has no fixed run to write")
	}
	unnoted := &EmitEvent{kind: evCall, seq: 20, call: emitCall{word: "print", pos: gpos(1)}}
	if _, _, ok := es.callRun(tree, unnoted, body, 0); ok {
		t.Error("a call whose arguments were never placed has no run")
	}
	short := call(21, 1, 0, lit(7))
	short.call.ops = nil
	if _, _, ok := es.callRun(tree, short, body, 0); ok {
		t.Error("sites that are not the call's operands place nothing")
	}
	// Inside a paren: the run at the paren's own level.
	inner := []core.Value{gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("print"), 2), gtok(core.NewString("a"), 8)}), 1)}
	if path, span, ok := es.callRun(tree, call(22, 2, 0, lit(8)), inner, 0); !ok || span != 2 || len(path) != 2 || path[1] != 0 {
		t.Errorf("a run inside a paren: %v %d %v", path, span, ok)
	}
}

func TestBarrierFreeAndWrittenOver(t *testing.T) {
	for _, c := range []struct {
		name string
		v    core.Value
		want bool
	}{
		{"a scalar literal", core.NewInteger(5), true},
		{"a list literal", core.NewEvalList([]core.Value{core.NewWord("f")}), true},
		{"a paren", core.NewParenExpr([]core.Value{core.NewWord("f")}), false},
		{"a word", core.NewWord("f"), false},
	} {
		if barrierFree(c.v) != c.want {
			t.Errorf("%s: barrierFree = %v", c.name, !c.want)
		}
	}
	written := []substPlan{{path: []int{0}, span: 2}}
	if !writtenOver(written, []int{1}) || writtenOver(written, []int{2}) || writtenOver(nil, []int{0}) {
		t.Error("a token inside a written run is gone from the island")
	}
	body := runBody()
	if inertBefore(body, 0, []int{3}) || !inertBefore(body, 0, []int{3}, written...) {
		t.Error("a word before the run is a barrier, unless the island writes it over")
	}
}

func TestRestartSubstsCallRuns(t *testing.T) {
	es := NewEmitState()
	body := []core.Value{
		gtok(core.NewWord("print"), 1), gtok(core.NewString("a"), 7), gtok(core.NewWord("print"), 11), gtok(core.NewString("b"), 17),
		gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("g"), 22)}), 21),
	}
	p1 := EmitEvent{kind: evCall, seq: 1, call: emitCall{word: "print", pos: gpos(1), ops: []EmitOperand{{kind: opConst}}}}
	p2 := EmitEvent{kind: evCall, seq: 2, call: emitCall{word: "print", pos: gpos(11), ops: []EmitOperand{{kind: opConst}}}}
	g := EmitEvent{kind: evCallUser, seq: 3, uc: emitUserCall{pos: gpos(22), nout: 1}}
	es.argSites = map[int][]argSite{}
	es.argSites[1] = []argSite{{pos: gpos(7), seq: -1}}
	es.argSites[2] = []argSite{{pos: gpos(17), seq: -1}}
	tree := rootTreeEvents([]EmitEvent{p1, p2, g}, false)
	got, ok := es.restartSubsts(tree, body, 0, []int{1, 2, 3})
	if !ok || len(got) != 3 || !got[0].none || got[0].span != 2 || !got[1].none || got[1].path[0] != 2 || got[2].none || got[2].seq != 3 {
		t.Errorf("each effect is written as nothing, the second past the first, and the paren as its value: %+v %v", got, ok)
	}
	lw := &lowerer{es: es, landingRoot: true}
	r := &landingRestart{token: 0, depth: 0, held: -1, substs: got[:2]}
	srcs, ok := lw.restartSubstSrcs(r, EmitOperand{}, -1)
	if !ok || len(srcs) != 2 || srcs[0].Src.Kind != RestartNone || srcs[1].Span != 2 || !srcs[0].Placed {
		t.Errorf("an effect's run is written as no token, a placed run: %+v %v", srcs, ok)
	}
	if !got[0].run || got[2].run {
		t.Errorf("a call run is placed, a paren's value is not a run: %+v", got)
	}
}

// TestClaimParked pins the residual's parked-result claim (NUR282's `j
// j`): the one result of a def-bound name's dispatch over an unknown callee
// is placed, claimed on the apply's spec (DynMethodSpec.Parks), where the
// walk lowered that apply; any other entry keeps its own rule.
func TestClaimParked(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, p: &Program{}}
	apply := func(seq, nout int, defRead bool) EmitEvent {
		return EmitEvent{kind: evCall, seq: seq, call: emitCall{word: "j", nout: nout, dynMethod: &DynMethodSpec{Word: "j", NOut: nout, DefRead: defRead}}}
	}
	es.frames[len(es.frames)-1] = []EmitEvent{apply(4, 1, true), apply(5, 1, false), apply(6, 2, true), {kind: evCallUser, seq: 7}}
	val := func(id string, seq, idx int) core.Value {
		v := core.NewInteger(1)
		v.ID = id
		if seq > 0 {
			es.producedBy[id] = producer{seq: seq, idx: idx}
		}
		return v
	}
	r := val("r", 4, 0)
	if lw.claimParked(r) {
		t.Error("an apply the walk never lowered claims nothing")
	}
	lw.p.DynMethods = []DynMethodSpec{{Word: "j"}}
	lw.dynMethodAt = map[int]int{4: 0}
	if !lw.claimParked(r) || !lw.p.DynMethods[0].Parks {
		t.Error("a def-bound name's dispatch result is placed, claimed on its apply")
	}
	for _, c := range []struct {
		name string
		v    core.Value
	}{
		{"a value no event produced", val("free", 0, 0)},
		{"a value of no identity", core.NewInteger(1)},
		{"a second result", val("second", 4, 1)},
		{"an event the frame does not hold", val("gone", 9, 0)},
		{"a member apply", val("member", 5, 0)},
		{"an apply of two results", val("pair", 6, 0)},
		{"a user call", val("user", 7, 0)},
	} {
		if lw.claimParked(c.v) {
			t.Errorf("%s: no claim", c.name)
		}
	}
	es.defReads = map[string]string{"r": "j"}
	if lw.claimParked(r) {
		t.Error("a result read back from a def dispatches: no claim")
	}
	delete(es.defReads, "r")
	es.memberFnReads = map[string]core.Value{"r": core.NewFunction(core.FnDefInfo{})}
	if lw.claimParked(r) {
		t.Error("a member read keeps its own rule")
	}
	delete(es.memberFnReads, "r")
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.Check.ReachSurvivorFnIDs = map[string]bool{"r": true}
	es.reg = reg
	if lw.claimParked(r) {
		t.Error("a reach group's survivor is re-stepped: no claim")
	}
}
