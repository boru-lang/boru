package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// flow_exit_test.go pins the lowering's share of NUR354 and NUR355: a
// counted loop publishes its index under the dynamic-environment mirror
// (publishesIndex), and the op a break/continue leaves a unit through records
// where the interpreter's tape stands after it (noteFlowExit, FlowExit).

func TestPublishesIndex(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es}
	counted := &emitLoop{iterName: "i"}
	if lw.publishesIndex(counted) {
		t.Error("no dynamic environment: the slot alone holds the index")
	}
	es.dynEnv = true
	if !lw.publishesIndex(counted) {
		t.Error("under the dynamic environment a counted loop publishes its index")
	}
	if lw.publishesIndex(&emitLoop{}) {
		t.Error("a condition loop has no index to publish")
	}
	if (&lowerer{}).publishesIndex(counted) {
		t.Error("no recorder: nothing to publish under")
	}
}

// flowBody is `f 1 (break 3) g x` at columns 1 3 5(6 12) 15 17, and a
// list arm `[h 7]` at 19(20 22).
func flowBody() []core.Value {
	return []core.Value{
		deoptTok("f", 1), deoptLit(core.NewInteger(1), 3),
		deoptParen(5, deoptTok("break", 6), deoptLit(core.NewInteger(3), 12)),
		deoptTok("g", 15), deoptTok("x", 17),
		deoptLit(flowArm(deoptTok("h", 20), deoptLit(core.NewInteger(7), 22)), 19),
	}
}

// flowArm is a list literal as the parser writes one: evaluated where it is
// not a word's code body.
func flowArm(items ...core.Value) core.Value {
	l := core.NewList(items)
	l.Eval = true
	return l
}

func TestNoteFlowExit(t *testing.T) {
	exits := map[int]FlowExit{}
	lw := &lowerer{landingBody: flowBody(), flowExits: &exits}
	lw.noteFlowExit(0, deoptAt(1), nil)                                   // `f` → the token after it, top level
	lw.noteFlowExit(1, deoptAt(1), []core.SrcPos{deoptAt(3), deoptAt(6)}) // `f 1 (…)` → `g`
	lw.noteFlowExit(2, deoptAt(6), nil)                                   // `break` in the paren → `3`, nested
	lw.noteFlowExit(3, deoptAt(12), nil)                                  // the paren's last → nothing after it
	lw.noteFlowExit(4, deoptAt(17), []core.SrcPos{deoptAt(19)})           // a run to the body's end
	lw.noteFlowExit(5, deoptAt(20), nil)                                  // inside a list literal
	for pc, want := range map[int]FlowExit{
		0: {Next: deoptAt(3), Top: true},
		1: {Next: deoptAt(15), Top: true},
		2: {Next: deoptAt(12)},
		3: {},
		4: {Top: true},
		5: {Next: deoptAt(22)},
	} {
		if got, ok := exits[pc]; !ok || got != want {
			t.Errorf("pc %d: got %+v (recorded %v), want %+v", pc, got, ok, want)
		}
	}
	// Nothing is recorded where no position names a body token, where the
	// token holding the position is not the op's own, while a loop of the
	// unit is open (it takes the signal), or with no table to record in.
	lw.noteFlowExit(10, core.SrcPos{}, nil)
	lw.noteFlowExit(11, deoptAt(2), nil)
	lw.loops = []loopCtx{{}}
	lw.noteFlowExit(12, deoptAt(1), nil)
	for _, pc := range []int{10, 11, 12} {
		if _, ok := exits[pc]; ok {
			t.Errorf("pc %d: recorded an exit where none stands", pc)
		}
	}
	(&lowerer{landingBody: flowBody()}).noteFlowExit(0, deoptAt(1), nil)
	// A first record makes the table.
	var fresh map[int]FlowExit
	(&lowerer{landingBody: flowBody(), flowExits: &fresh}).noteFlowExit(7, deoptAt(15), nil)
	if fresh[7] != (FlowExit{Next: deoptAt(17), Top: true}) {
		t.Errorf("a first record: got %+v", fresh)
	}
}

// TestNoteCallFlowExit: only a native call op that may run a body records,
// at the op lowerCall just emitted.
func TestNoteCallFlowExit(t *testing.T) {
	exits := map[int]FlowExit{}
	var code []Instr
	lw := &lowerer{landingBody: flowBody(), flowExits: &exits, code: &code}
	poly := &emitCall{poly: true, pos: deoptAt(15)}
	lw.noteCallFlowExit(0, poly) // nothing emitted yet
	code = []Instr{{Op: OpMakeList}}
	lw.noteCallFlowExit(0, poly) // not a native call op
	code = append(code, Instr{Op: OpCallNative})
	lw.noteCallFlowExit(0, &emitCall{pos: deoptAt(15)}) // no signature: runs nothing
	if len(exits) != 0 {
		t.Fatalf("recorded %v where no native body runner stands", exits)
	}
	code = append(code, Instr{Op: OpCallNativePoly})
	lw.noteCallFlowExit(0, poly)
	if exits[2] != (FlowExit{Next: deoptAt(17), Top: true}) {
		t.Errorf("a poly call: got %v", exits)
	}
}

func TestCallFlowOps(t *testing.T) {
	if (&lowerer{}).callFlowOps(0) != nil {
		t.Error("no recorder: no operand sites")
	}
	es := NewEmitState()
	es.argSites = map[int][]argSite{
		3: {{pos: deoptAt(3), seq: -1}, {seq: 2}, {seq: 9}},
		2: {{pos: deoptAt(7), seq: -1}},
	}
	lw := &lowerer{es: es, scopes: [][]EmitEvent{{deoptCall(2, "g", 5)}}}
	got := lw.callFlowOps(3)
	want := []core.SrcPos{deoptAt(3), deoptAt(5), deoptAt(7)}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("site %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestFallbackFlowOps(t *testing.T) {
	es := NewEmitState()
	read := deoptLit(core.NewList(nil), 30)
	read.ID = "fb-read"
	es.readPos = map[string]core.SrcPos{"fb-read": deoptAt(9)}
	es.fallbacks = []core.FallbackSpan{{Tokens: []core.Value{deoptTok("do", 7), read}}}
	got := (&lowerer{es: es}).fallbackFlowOps(&emitFallback{spanIdx: 0})
	if len(got) != 2 || got[0] != deoptAt(7) || got[1] != deoptAt(9) {
		t.Errorf("a word-read island operand stands where the read was written: got %v", got)
	}
}

// TestStoredHandlerDepsDeep pins the runtime stamp's snapshot: the names the
// body reads and the names the user fns it calls read, transitively, once
// each — a recursive fn ends the walk — and nothing through a word that is no
// user fn.
func TestStoredHandlerDepsDeep(t *testing.T) {
	var nilES *EmitState
	if d := nilES.storedHandlerDepsDeep([]core.Value{core.NewWord("x")}); len(d) != 0 {
		t.Fatalf("nil EmitState: %v", d)
	}
	r := runUnitReg(t)
	r.Defs.Push("zzdeep-k", core.NewInteger(1))
	r.Register("zzdeep-leaf", core.Signature{Impl: core.Boru([]core.Value{core.NewWord("zzdeep-k")}), BarrierPos: -1})
	r.Register("zzdeep-mid", core.Signature{Impl: core.Boru([]core.Value{core.NewWord("zzdeep-leaf"), core.NewWord("zzdeep-mid"), core.NewWord("add")}), BarrierPos: -1})
	es := NewEmitState()
	es.reg = r
	deps := es.storedHandlerDepsDeep([]core.Value{core.NewWord("zzdeep-mid"), core.NewWord("zzdeep-unbound")})
	for _, name := range []string{"zzdeep-mid", "zzdeep-leaf", "zzdeep-k"} {
		if !deps[name] {
			t.Errorf("deps %v: want %s", deps, name)
		}
	}
	if deps["zzdeep-unbound"] || deps["add"] {
		t.Errorf("deps %v: an unbound word or a native is no dependency", deps)
	}
}
