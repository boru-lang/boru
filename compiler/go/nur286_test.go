package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur286_test.go pins the root landing's guard (NUR286): a landing whose OWN
// step had values beneath it (landingOwn — the first note, never a def-bound
// read's re-step, which is the read's) carries LandingBeneathGuard unless the
// interpreter parks the value (callResultPlaced), the residual's apply
// re-steps it as the interpreter's re-step does (residualApplies), or an
// event applies it (eventApplies).

func TestLandingOwn(t *testing.T) {
	es, v := nfState()
	es.NoteLandingNext(v, core.LandingNextBoundary, true, core.Value{})
	es.NoteLandingNext(v, core.LandingNextValue, false, core.Value{})
	if got := es.landingOwn[1]; !got.beneath || got.next != core.LandingNextBoundary || got.id != "v" {
		t.Errorf("the first note at the value's own step is its own: %+v", got)
	}
	read, rv := nfState()
	read.defReads = map[string]string{"v": "j"}
	read.NoteLandingNext(rv, core.LandingNextBoundary, true, core.Value{})
	if _, own := read.landingOwn[1]; own || !read.landingBeneath[1] {
		t.Error("a def-bound read's re-step over values beneath is the read's, not the landing's own")
	}
}

func TestNoteRootBeneathLanding(t *testing.T) {
	es := NewEmitState()
	es.landingOwn = map[int]landingStep{3: {beneath: true}, 4: {}}
	lw := &lowerer{es: es, landingRoot: true}
	lw.noteRootBeneathLanding(7, 4)
	if len(lw.rootBeneathLandings) != 0 {
		t.Error("a landing with nothing beneath at its own step is not recorded")
	}
	lw.depth = 1
	lw.noteRootBeneathLanding(7, 3)
	lw.depth, lw.landingRoot = 0, false
	lw.noteRootBeneathLanding(7, 3)
	if len(lw.rootBeneathLandings) != 0 {
		t.Error("a fragment's landing keeps today's arms")
	}
	lw.landingRoot = true
	lw.noteRootBeneathLanding(7, 3)
	if len(lw.rootBeneathLandings) != 1 || lw.rootBeneathLandings[0] != [2]int{7, 3} {
		t.Errorf("a root landing over its own values is recorded: %v", lw.rootBeneathLandings)
	}
	unit := &lowerer{es: es, isFnUnit: true}
	unit.noteRootBeneathLanding(8, 3)
	if len(unit.rootBeneathLandings) != 1 {
		t.Errorf("a fn unit's own-depth landing over its own values is recorded: %v", unit.rootBeneathLandings)
	}
}

// TestGuardUnitLandings pins the fn unit's twin of the root guard (NUR286's
// fn-body form): a landing over the unit's own values is guarded unless an
// event applies the value, the unit's whole-frame replay re-steps it (by the
// event, or by the slot it was promoted to), or the unit carries a tail apply
// of its own, which keeps today's arms.
func TestGuardUnitLandings(t *testing.T) {
	es := NewEmitState()
	es.landingOwn = map[int]landingStep{3: {beneath: true, id: "a"}, 4: {beneath: true, id: "b"}, 5: {beneath: true, id: "c"}}
	code := []Instr{{Op: OpReStepLanding}, {Op: OpReStepLanding}, {Op: OpReStepLanding}}
	flw := &lowerer{es: es, isFnUnit: true, code: &code, promoted: map[int]int{4: 2},
		rootBeneathLandings: [][2]int{{0, 3}, {1, 4}, {2, 5}}}
	rec := &fnUnitRec{frag: &EmitFragment{}, dynFrameW: 2,
		outOps: []EmitOperand{{kind: opConst}, {kind: opLocal, idx: 2}, {kind: opEvent, idx: 5}}}
	es.guardUnitLandings(flw, rec)
	if code[0].Arg&LandingBeneathGuard == 0 || code[1].Arg&LandingBeneathGuard != 0 || code[2].Arg&LandingBeneathGuard != 0 {
		t.Errorf("only the landing no replay re-steps is guarded: %+v", code)
	}
	code = []Instr{{Op: OpReStepLanding}, {Op: OpReStepLanding}, {Op: OpReStepLanding}}
	es.guardUnitLandings(flw, &fnUnitRec{frag: &EmitFragment{}, dynTrailArity: 1})
	if code[0].Arg != 0 {
		t.Error("a unit with a trailing apply of its own keeps today's arms")
	}
	if frameReplays(&fnUnitRec{dynFrameW: 1, outOps: []EmitOperand{{kind: opEvent, idx: 3}, {kind: opConst}}}, 3, nil) {
		t.Error("an entry below the replayed top is not re-stepped by it")
	}
}

// n286State is a root with the landed event seq 1 (its value id "v") and a
// constant beneath (id "k").
func n286State() (*EmitState, core.Value, core.Value) {
	es, v := nfState()
	k := core.NewInteger(5)
	k.ID = "k"
	return es, v, k
}

func TestResidualApplies(t *testing.T) {
	es, v, k := n286State()
	word := core.NewWord("add")
	read := v
	read.ID = "r"
	es.producedBy["r"] = producer{seq: 1}
	es.defReads = map[string]string{"r": "j"}
	for name, c := range map[string]struct {
		dynOp    Opcode
		residual []core.Value
		want     bool
	}{
		"no apply":                            {0, []core.Value{k, v}, false},
		"the trailing apply's lead":           {OpCallDynamicTrailing, []core.Value{v, k}, true},
		"not the trailing lead":               {OpCallDynamicTrailing, []core.Value{k, v}, false},
		"an empty trailing residual":          {OpCallDynamicTrailing, nil, false},
		"a leading lead with nothing above":   {OpCallDynamic, []core.Value{v}, true},
		"a leading lead with args above":      {OpCallDynamic, []core.Value{v, k}, false},
		"a window's last entry":               {OpCallDynamicMixed, []core.Value{k, v}, true},
		"a window entry under a word":         {OpCallDynamicMixed, []core.Value{k, v, word, k}, true},
		"a window entry under a value":        {OpCallDynamicMixed, []core.Value{v, k}, false},
		"absent from the window":              {OpCallDynamicMixed, []core.Value{k}, false},
		"a def-bound read of it":              {OpCallDynamicTrailing, []core.Value{read, k}, false},
		"a def-bound read in a mark's window": {OpCallDynMixedFromMark, []core.Value{k, read}, false},
	} {
		if got := es.residualApplies(c.dynOp, c.residual, 1); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	es.landingOwn = map[int]landingStep{1: {next: core.LandingNextValue}}
	if !es.residualApplies(OpCallDynamicMixed, []core.Value{k, v, k}, 1) {
		t.Error("a window entry whose own step collected a value-bound word: the island collects it as the interpreter did")
	}
}

func TestEventApplies(t *testing.T) {
	apply := func(op EmitOperand) EmitEvent {
		return EmitEvent{seq: 5, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, dynApply: 1, ops: []EmitOperand{op}}}
	}
	takes := EmitEvent{seq: 4, kind: evCall, call: emitCall{word: "typeof", nout: 1, ops: []EmitOperand{EventOperand(1, 0)}}}
	if eventApplies([]EmitEvent{takes}, 1, nil) {
		t.Error("a plain word takes the value, it does not apply it")
	}
	if !eventApplies([]EmitEvent{takes, apply(EventOperand(1, 0))}, 1, nil) {
		t.Error("a dynamic apply of the value re-steps it")
	}
	if eventApplies([]EmitEvent{apply(EventOperand(2, 0))}, 1, nil) {
		t.Error("an apply of another value is not this one's")
	}
	mixed := EmitEvent{seq: 6, kind: evCall, call: emitCall{dynMixed: true, ops: []EmitOperand{localOperand(3)}}}
	if !eventApplies([]EmitEvent{mixed}, 1, map[int]int{1: 3}) {
		t.Error("a mixed window over the value's promoted slot re-steps it")
	}
	armed := EmitEvent{seq: 7, kind: evBranch, br: &emitBranch{then: &EmitFragment{events: []EmitEvent{apply(EventOperand(1, 0))}}}}
	if !eventApplies([]EmitEvent{armed}, 1, nil) {
		t.Error("an apply inside a fragment re-steps it too")
	}
}

func TestGuardRootLandings(t *testing.T) {
	es, v, k := n286State()
	code := []Instr{{Op: OpReStepLanding, Arg: 1}}
	lw := &lowerer{es: es, code: &code, rootBeneathLandings: [][2]int{{0, 1}}}
	es.guardRootLandings(lw, OpCallDynamicTrailing, []core.Value{v, k})
	if code[0].Arg != 1 {
		t.Error("the trailing apply's lead is the landing's partner: no guard")
	}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 2, kind: evCall, call: emitCall{dynApply: 1, ops: []EmitOperand{EventOperand(1, 0)}}})
	es.guardRootLandings(lw, 0, []core.Value{k})
	if code[0].Arg != 1 {
		t.Error("an event that applies the value is the landing's partner: no guard")
	}
	es.frames[0] = es.frames[0][:1]
	es.landingOwn = map[int]landingStep{1: {beneath: true, id: "v"}}
	es.guardRootLandings(lw, 0, []core.Value{v, k})
	if code[0].Arg != 1|LandingBeneathGuard {
		t.Errorf("no apply re-steps the value: guarded, got %d", code[0].Arg)
	}
	placed, pv, pk := n286State()
	placed.frames[0][0] = EmitEvent{seq: 1, kind: evCallUser, uc: emitUserCall{nout: 1}}
	placed.landingOwn = map[int]landingStep{1: {beneath: true, id: "v"}}
	pcode := []Instr{{Op: OpReStepLanding}}
	plw := &lowerer{es: placed, code: &pcode, rootBeneathLandings: [][2]int{{0, 1}}}
	placed.guardRootLandings(plw, 0, []core.Value{pk, pv})
	if pcode[0].Arg != 0 {
		t.Error("a user call's result the interpreter parks is re-stepped by nobody: no guard")
	}
}

func TestClosureTakesArgs(t *testing.T) {
	one := CompiledFn{Name: "fnval$body", Lambda: true, NArgs: 1, Params: []*core.Type{core.TInteger}}
	none := CompiledFn{Name: "fnval$body", Lambda: true}
	prog := &Program{Fns: []CompiledFn{one, none, {Name: "do$body", NArgs: 1}}}
	if ClosureTakesArgs(core.NewInteger(1)) {
		t.Error("a non-closure value takes nothing here")
	}
	if !ClosureTakesArgs(NewClosure(prog, 0, nil)) {
		t.Error("a fn value's closure with a parameter takes arguments")
	}
	if ClosureTakesArgs(NewClosure(prog, 1, nil)) {
		t.Error("a nullary fn value's closure takes none")
	}
	if ClosureTakesArgs(NewClosure(prog, 2, nil)) {
		t.Error("a callback body unit's closure is no fn value")
	}
}
