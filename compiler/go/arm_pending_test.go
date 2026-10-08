package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// arm_pending_test.go pins NUR356's two walks over synthetic event lists:
// pendingArmWalk (an `if` arm's pending literal, arm_pending.go) and
// eagerLiteralRefusal (a literal a call matched at run time takes,
// eager_literal.go). The lang suite's nur356 tests prove them end to end.

// armPendingBranch is a branch event at seq whose arms left residue p.
func armPendingBranch(seq int, p core.PendingResidue) EmitEvent {
	return EmitEvent{seq: seq, kind: evBranch, br: &emitBranch{pending: p}}
}

// readsX is the residue of a literal that reads x and calls nothing.
var readsX = core.PendingResidue{Left: true, Reads: []string{"x"}}

// callsLit is the residue of a literal that calls.
var callsLit = core.PendingResidue{Left: true, Calls: true}

func TestPendingArmWalk(t *testing.T) {
	es := NewEmitState()
	es.bindTwins = []core.BindTransition{{Name: "x"}, {Name: "y"}}
	size := &core.Signature{Args: []*core.Type{core.TAny}}
	consume := EmitEvent{seq: 2, kind: evCall, call: emitCall{word: "size", sig: size, ops: []EmitOperand{EventOperand(1, 0)}}}
	for _, tc := range []struct {
		name   string
		events []EmitEvent
		open   bool
		reason bool
	}{
		{"nothing pending", []EmitEvent{consume}, false, false},
		{"pending to the end", []EmitEvent{armPendingBranch(1, readsX)}, true, false},
		{"consumed at once", []EmitEvent{armPendingBranch(1, readsX), consume}, false, false},
		{"a def of it (twin, then its bind)", []EmitEvent{armPendingBranch(1, readsX),
			{seq: 2, kind: evBindTwin, twin: &emitBindTwin{idx: 1, pos: deoptAt(3)}},
			{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: 1, pos: deoptAt(3)}}}, false, false},
		{"a rebind of a name it reads", []EmitEvent{armPendingBranch(1, readsX),
			{seq: 2, kind: evDynBind, dyn: &emitDynBind{name: "x", srcSeq: -1}}}, false, true},
		{"a bind of another name, then the end", []EmitEvent{armPendingBranch(1, readsX),
			{seq: 2, kind: evBindTwin, twin: &emitBindTwin{idx: 1}},
			{seq: 3, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: -1}}}, true, false},
		{"a bind of an unread name, the calling literal's arms quiet", []EmitEvent{armPendingBranch(1, callsLit),
			{seq: 2, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: -1}}}, true, false},
		{"a bind when the calling literal's arm prints", []EmitEvent{{seq: 1, kind: evBranch, br: &emitBranch{pending: callsLit,
			then: &EmitFragment{events: []EmitEvent{{kind: evCall, call: emitCall{word: "print", sig: &core.Signature{}}}}}}},
			{seq: 2, kind: evDynBind, dyn: &emitDynBind{name: "y", srcSeq: -1}}}, false, true},
		{"many values, one consumer", []EmitEvent{
			{seq: 1, kind: evBranch, br: &emitBranch{pending: readsX, then: &EmitFragment{residualN: 2}}}, consume}, false, true},
		{"a nested refusal", []EmitEvent{{seq: 1, kind: evBranch, br: &emitBranch{
			condFrag: &EmitFragment{events: []EmitEvent{armPendingBranch(4, readsX)}}}}}, false, true},
	} {
		open, reason := es.pendingArmWalk(tc.events)
		if open.Left != tc.open || (reason != "") != tc.reason {
			t.Errorf("%s: open=%v reason=%q", tc.name, open.Left, reason)
		}
	}
	if r := es.pendingArmRefusal([]EmitEvent{armPendingBranch(1, readsX), {seq: 2, kind: evTrap}}, nil, nil); !strings.Contains(r, "NUR356") {
		t.Errorf("a trap before the literal's evaluation declines: %q", r)
	}
	rec := &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{armPendingBranch(1, readsX), {seq: 2, kind: evTrap}}}}
	if r := es.lowerUnitEvents(&lowerer{}, rec); !strings.Contains(r, "NUR356") {
		t.Errorf("a unit's refusal is its lowering's: %q", r)
	}
	if r := es.pendingArmRefusal([]EmitEvent{armPendingBranch(1, readsX)}, nil, nil); r != "" {
		t.Errorf("a sequence whose end evaluates it compiles: %q", r)
	}
}

func TestEventLeavesPending(t *testing.T) {
	es := NewEmitState()
	nested := func(p core.PendingResidue) *EmitFragment {
		return &EmitFragment{events: []EmitEvent{armPendingBranch(9, p)}}
	}
	refusing := &EmitFragment{events: []EmitEvent{armPendingBranch(8, readsX), {seq: 9, kind: evTrap}}}
	for _, tc := range []struct {
		name   string
		ev     EmitEvent
		left   bool
		reason bool
	}{
		{"an arm's own", armPendingBranch(1, readsX), true, false},
		{"carried up from a nested arm", EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{then: nested(readsX), els: &EmitFragment{}}}, true, false},
		{"a case chain's arms evaluate it", EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{then: nested(readsX), sweptArms: true}}, false, false},
		{"an arm that refuses", EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{els: refusing}}, false, true},
		{"a condition left pending", EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{condFrag: nested(readsX)}}, false, true},
		{"a loop body's end evaluates it", EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{body: nested(readsX)}}, false, false},
		{"a loop body that refuses", EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{body: refusing}}, false, true},
		{"a loop condition left pending", EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{cond: nested(readsX)}}, false, true},
		{"a bodiless loop", EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{}}, false, false},
		{"a call", EmitEvent{seq: 1, kind: evCall}, false, false},
	} {
		left, reason := es.eventLeavesPending(&tc.ev)
		if left.Left != tc.left || (reason != "") != tc.reason {
			t.Errorf("%s: left=%v reason=%q", tc.name, left.Left, reason)
		}
	}
	if r := es.pendingArmCond(nil); r != "" {
		t.Errorf("no condition: %q", r)
	}
	if r := es.pendingArmCond(refusing); r == "" {
		t.Error("a condition whose own walk refuses declines")
	}
	if r := es.pendingArmCond(&EmitFragment{}); r != "" {
		t.Errorf("a plain condition: %q", r)
	}
}

func TestPendingUntouched(t *testing.T) {
	es := NewEmitState()
	es.bindTwins = []core.BindTransition{{Name: "x"}, {Name: "y"}}
	for _, tc := range []struct {
		name string
		ev   EmitEvent
		p    core.PendingResidue
		want bool
	}{
		{"calls admit nothing", EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "y"}}, callsLit, false},
		{"a bind of another name", EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "y"}}, readsX, true},
		{"a bind of a read name", EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "x"}}, readsX, false},
		{"a twin of another name", EmitEvent{kind: evBindTwin, twin: &emitBindTwin{idx: 1}}, readsX, true},
		{"a twin of a read name", EmitEvent{kind: evBindTwin, twin: &emitBindTwin{idx: 0}}, readsX, false},
		{"a twin off the ledger", EmitEvent{kind: evBindTwin, twin: &emitBindTwin{idx: 7}}, readsX, false},
		{"a pure read", EmitEvent{kind: evCall, call: emitCall{word: "get"}}, readsX, true},
		{"an effect", EmitEvent{kind: evCall, call: emitCall{word: "print"}}, readsX, false},
	} {
		if got := es.pendingUntouched(&tc.ev, tc.p, false); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}

func TestConsumesPendingEvaluated(t *testing.T) {
	sig := &core.Signature{Args: []*core.Type{core.TAny}}
	raw := &core.Signature{Args: []*core.Type{core.TAny}, NoEvalArgs: map[int]bool{0: true}}
	reads := []EmitOperand{EventOperand(1, 0)}
	for _, tc := range []struct {
		name string
		ev   EmitEvent
		want bool
	}{
		{"a committed call", EmitEvent{kind: evCall, call: emitCall{sig: sig, ops: reads}}, true},
		{"an assembly", EmitEvent{kind: evCall, call: emitCall{makeList: true, ops: reads}}, true},
		{"a call of something else", EmitEvent{kind: evCall, call: emitCall{sig: sig, ops: []EmitOperand{EventOperand(2, 0)}}}, false},
		{"a poly", EmitEvent{kind: evCall, call: emitCall{sig: sig, poly: true, ops: reads}}, false},
		{"a raw slot", EmitEvent{kind: evCall, call: emitCall{sig: raw, ops: reads}}, false},
		{"a user call", EmitEvent{kind: evCallUser, uc: emitUserCall{ops: reads}}, true},
		{"a user call that may miss", EmitEvent{kind: evCallUser, uc: emitUserCall{ops: reads, mayMiss: true}}, false},
		{"a def", EmitEvent{kind: evDynBind, dyn: &emitDynBind{srcSeq: 1}}, true},
		{"a store", EmitEvent{kind: evStore, store: &emitStore{src: EventOperand(1, 0)}}, true},
		{"a store of a local", EmitEvent{kind: evStore, store: &emitStore{src: localOperand(0)}}, false},
		{"a trap", EmitEvent{kind: evTrap}, false},
	} {
		if got := consumesPendingEvaluated(&tc.ev, 1); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
	if bindsPendingDef([]EmitEvent{{kind: evBindTwin, twin: &emitBindTwin{}}}, 0, 1) {
		t.Error("a twin with nothing after it binds no def of the value")
	}
	if bindsPendingDef([]EmitEvent{{kind: evCall}}, 0, 1) {
		t.Error("a call is no twin")
	}
}

func TestEagerLiteralRefusal(t *testing.T) {
	es := NewEmitState()
	h := EmitEvent{seq: 1, kind: evCallUser, uc: emitUserCall{unit: -1}}
	// The literal's element run records one level deeper than the call.
	print := EmitEvent{seq: 2, kind: evCall, litDepth: 1, call: emitCall{word: "print", sig: &core.Signature{}}}
	poly := EmitEvent{seq: 3, kind: evCall, call: emitCall{word: "each", poly: true}}
	if r := es.eagerLiteralRefusal([]EmitEvent{h, print, poly}, nil, nil); !strings.Contains(r, "NUR356") {
		t.Errorf("an effect in a literal a poly takes declines: %q", r)
	}
	br := EmitEvent{seq: 4, kind: evBranch, br: &emitBranch{then: &EmitFragment{events: []EmitEvent{print, poly}}, els: nil}}
	if r := es.eagerLiteralRefusal([]EmitEvent{br}, nil, nil); r == "" {
		t.Error("a nested sequence is walked")
	}
	committed := poly
	committed.call.poly = false
	if r := es.eagerLiteralRefusal([]EmitEvent{h, print, committed}, nil, nil); r != "" {
		t.Errorf("a committed call takes it as the interpreter does: %q", r)
	}
	if r := es.eagerLiteralRefusal([]EmitEvent{print, h, poly}, nil, nil); r != "" {
		t.Errorf("the block ends at the call's other operand: %q", r)
	}
	shallow := print
	shallow.litDepth = 0
	if r := es.eagerLiteralRefusal([]EmitEvent{h, shallow, poly}, nil, nil); r != "" {
		t.Errorf("an effect at the call's own depth is no assembly of its literal: %q", r)
	}
	add := EmitEvent{seq: 2, kind: evCall, litDepth: 1, call: emitCall{word: "add", nout: 1, sig: &core.Signature{}}}
	asm := EmitEvent{seq: 5, kind: evCall, call: emitCall{makeList: true, nout: 1}}
	if r := es.eagerLiteralRefusal([]EmitEvent{h, add, asm, poly}, nil, nil); r != "" {
		t.Errorf("a quiet assembly: %q", r)
	}
	if r := es.eagerLiteralRefusal([]EmitEvent{h, print, asm, poly}, nil, nil); r == "" {
		t.Error("an effect before the assembly declines")
	}
}

func TestMayEffect(t *testing.T) {
	r := seam7Reg(t)
	r.Register("pureword", core.Signature{Args: []*core.Type{core.TAny}})
	r.Register("bodyword", core.Signature{Args: []*core.Type{core.TAny}, CompileEffect: core.CompileFallbackBody})
	es := NewEmitState()
	es.BindRegistry(r)
	es.bindTwins = []core.BindTransition{{Name: "x"}}
	sig := &core.Signature{}
	body := &core.Signature{CompileEffect: core.CompileFallbackBody}
	quietUnit := &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{
		{kind: evDynBind, dyn: &emitDynBind{name: "x"}},
		{kind: evCallUser, uc: emitUserCall{unit: 0}},
		{kind: evCall, call: emitCall{nout: 1, sig: sig}},
	}}}
	loudUnit := &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{{kind: evCall, call: emitCall{word: "print", sig: sig}}}}}
	leakUnit := &fnUnitRec{keepsDefs: true, frag: &EmitFragment{events: []EmitEvent{{kind: evDynBind, dyn: &emitDynBind{name: "x"}}}}}
	es.fnRecs = []*fnUnitRec{quietUnit, loudUnit, {}, leakUnit}
	closure := func(u int) EmitOperand { return EmitOperand{kind: opClosure, closureUnit: u} }
	readsX := func(n string) bool { return n == "x" }
	for _, tc := range []struct {
		name  string
		ev    EmitEvent
		binds func(string) bool
		want  bool
	}{
		{"a value word", EmitEvent{kind: evCall, call: emitCall{nout: 1, sig: sig}}, anyBind, false},
		{"an assembly", EmitEvent{kind: evCall, call: emitCall{makeMap: true}}, anyBind, false},
		{"a word called for its effect", EmitEvent{kind: evCall, call: emitCall{sig: sig}}, anyBind, true},
		{"a code-body word over a quiet body", EmitEvent{kind: evCall, call: emitCall{nout: 1, sig: body, ops: []EmitOperand{closure(0)}}}, anyBind, false},
		{"a code-body word over a loud body", EmitEvent{kind: evCall, call: emitCall{nout: 1, sig: body, ops: []EmitOperand{ConstOperand(0), closure(1)}}}, anyBind, true},
		{"a code-body word over an interpreted body", EmitEvent{kind: evCall, call: emitCall{nout: 1, sig: body}}, anyBind, true},
		{"a poly value word", EmitEvent{kind: evCall, call: emitCall{word: "pureword", nout: 1, poly: true}}, anyBind, false},
		{"a poly code-body word", EmitEvent{kind: evCall, call: emitCall{word: "bodyword", nout: 1, poly: true}}, anyBind, true},
		{"a poly of an unknown word", EmitEvent{kind: evCall, call: emitCall{word: "nope", nout: 1, poly: true, polyReg: r}}, anyBind, true},
		{"an unknown signature", EmitEvent{kind: evCall, call: emitCall{nout: 1}}, anyBind, true},
		{"a dynamic apply", EmitEvent{kind: evCall, call: emitCall{nout: 1, sig: sig, dynApply: 1}}, anyBind, true},
		{"a quiet fn (recursion included)", EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0}}, anyBind, false},
		{"a fn that prints", EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 1}}, anyBind, true},
		{"a fn not compiled", EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 2}}, anyBind, true},
		{"a fn leaking a def", EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 3}}, anyBind, true},
		{"a fn leaking a def nothing reads", EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 3}}, func(string) bool { return false }, false},
		{"a user poly", EmitEvent{kind: evCallUser, uc: emitUserCall{unit: -1}}, anyBind, true},
		{"a branch of reads", EmitEvent{kind: evBranch, br: &emitBranch{then: &EmitFragment{events: []EmitEvent{{kind: evCall, call: emitCall{nout: 1, sig: sig}}}}}}, anyBind, false},
		{"a loop that prints", EmitEvent{kind: evLoop, loop: &emitLoop{body: &EmitFragment{events: []EmitEvent{{kind: evCall, call: emitCall{sig: sig}}}}}}, anyBind, true},
		{"a read name's bind", EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "x"}}, readsX, true},
		{"another name's bind", EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "y"}}, readsX, false},
		{"a read name's twin", EmitEvent{kind: evBindTwin, twin: &emitBindTwin{idx: 0}}, readsX, true},
		{"a twin off the ledger", EmitEvent{kind: evBindTwin, twin: &emitBindTwin{idx: 4}}, readsX, true},
		{"a store", EmitEvent{kind: evStore, store: &emitStore{}}, anyBind, true},
		{"a trap", EmitEvent{kind: evTrap}, anyBind, true},
	} {
		if got := es.mayEffect(&tc.ev, effectScope{seen: map[int]bool{}, binds: tc.binds}); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
	es.dynScopeNames = map[string]bool{"d": true}
	if !es.dynReadName("d") || es.dynReadName("e") {
		t.Error("dynReadName")
	}
	// A literal that calls admits an effect-free event between, where its
	// branch's arms have none: neither can observe the other.
	calls := core.PendingResidue{Left: true, Calls: true}
	get := EmitEvent{kind: evCall, call: emitCall{word: "get"}}
	if !es.pendingUntouched(&get, calls, true) || es.pendingUntouched(&get, calls, false) {
		t.Error("quiet arms admit a quiet event; others do not")
	}
	leak := EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 3}}
	if es.pendingUntouched(&leak, core.PendingResidue{Left: true, Reads: []string{"x"}}, false) {
		t.Error("a call leaking a def of a name the literal reads touches it")
	}
	es.dynScopeNames = map[string]bool{"x": true}
	if es.pendingUntouched(&leak, calls, true) {
		t.Error("a call leaking a def of a name compiled code reads at run time touches it")
	}
}

func TestInlineLitDepth(t *testing.T) {
	es := NewEmitState()
	if es.inlineLitDepth() != 0 {
		t.Error("no region is open")
	}
	es.PushInlineCtxBoundary()
	es.PushInlineCtxBoundary()
	es.openUnitRecs = append(es.openUnitRecs, 0)
	es.PushInlineCtxBoundary()
	if got := es.inlineLitDepth(); got != 1 {
		t.Errorf("only the regions this unit opened count: %d", got)
	}
	es.openUnitRecs = nil
	if got := es.inlineLitDepth(); got != 2 {
		t.Errorf("the outer unit's regions: %d", got)
	}
	m := core.NewOrderedMap()
	m.Set("a", deoptTok("y", 42))
	mp := deoptLit(core.NewMap(m), 40)
	mp.Eval = true
	if v, ok := literalTokenAt([]core.Value{mp}, deoptAt(40)); !ok || v.Pos() != deoptAt(40) {
		t.Error("a pending map literal")
	}
	mp.Eval = false
	if _, ok := literalTokenAt([]core.Value{mp}, deoptAt(40)); ok {
		t.Error("a map not marked pending is no pending literal")
	}
}

func TestRuntimeMatchedAndContract(t *testing.T) {
	for name, tc := range map[string]struct {
		ev   EmitEvent
		want bool
	}{
		"a poly":                    {EmitEvent{kind: evCall, call: emitCall{poly: true}}, true},
		"a committed call":          {EmitEvent{kind: evCall}, false},
		"a user call that fits":     {EmitEvent{kind: evCallUser}, false},
		"a user call that may miss": {EmitEvent{kind: evCallUser, uc: emitUserCall{mayMiss: true}}, true},
		"a rematch trap":            {EmitEvent{kind: evTrap, trap: EmitTrap{rematchOps: []EmitOperand{ConstOperand(0)}}}, true},
		"a plain trap":              {EmitEvent{kind: evTrap}, false},
		"an apply of a runtime fn":  {EmitEvent{kind: evCall, call: emitCall{dynApply: 1, ops: []EmitOperand{EventOperand(1, 0)}}}, true},
		"an apply of a closure":     {EmitEvent{kind: evCall, call: emitCall{dynApply: 1, ops: []EmitOperand{{kind: opClosure}}}}, false},
		"an apply of no operand":    {EmitEvent{kind: evCall, call: emitCall{dynApply: 1}}, true},
		"a def":                     {EmitEvent{kind: evDynBind, dyn: &emitDynBind{}}, false},
	} {
		if got := NewEmitState().runtimeMatched(&tc.ev); got != tc.want {
			t.Errorf("%s: %v", name, got)
		}
	}
	fnEs := NewEmitState()
	fnEs.consts = []core.Value{core.NewCarrier(core.TFunction), core.NewInteger(1)}
	if !fnEs.knownFnLead([]EmitOperand{ConstOperand(0)}) || fnEs.knownFnLead([]EmitOperand{ConstOperand(1)}) || fnEs.knownFnLead([]EmitOperand{ConstOperand(5)}) {
		t.Error("a constant fn lead is known; a constant non-fn or unknown one is not")
	}
	pat := core.NewInteger(1)
	rec := &fnUnitRec{paramTypes: []*core.Type{core.TInteger, nil}}
	if contractMayMiss(rec, []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TAny)}) {
		t.Error("an Integer to an Integer param, anything to an untyped one")
	}
	if !contractMayMiss(rec, []core.Value{core.NewDynamicCarrier(core.TAny)}) {
		t.Error("a gradual value may miss")
	}
	if contractMayMiss(&fnUnitRec{paramTypes: []*core.Type{core.TAny}}, []core.Value{core.NewDynamicCarrier(core.TAny)}) {
		t.Error("a gradual value to an Any param cannot miss")
	}
	if !contractMayMiss(rec, []core.Value{core.NewCarrier(core.TList)}) {
		t.Error("a wider type may miss")
	}
	rec.paramPatterns = []*core.Value{&pat}
	if !contractMayMiss(rec, []core.Value{core.NewCarrier(core.TInteger)}) {
		t.Error("a pattern is checked against the value")
	}
}
