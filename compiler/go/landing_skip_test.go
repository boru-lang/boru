package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestSealLandingSkip pins the lowering half of NUR190's `/q` claim
// (2026-09-26): a landing with a function word after it gets its CLAIM
// target (LandingWord.Skip) only when the residual apply just emitted is
// the one laid over that word — the landing op, then the word's
// argument-free one-result call, then OpCallDynamic /1 applying the landed
// value over it. Every other shape seats nothing, and the VM's claim keeps
// its loud defer.
func TestSealLandingSkip(t *testing.T) {
	// seq 1 is the landed read, seq 2 the word's call, seq 3 another event.
	userCall := EmitEvent{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}}
	nativeCall := EmitEvent{seq: 2, kind: evCall, call: emitCall{word: "z", nout: 1}}
	build := func(word Instr, ev EmitEvent) (*lowerer, map[int]LandingWord) {
		es := NewEmitState()
		es.frames = [][]EmitEvent{{{seq: 1, kind: evCall}, ev}}
		es.fnRecs = []*fnUnitRec{{name: "z"}}
		code := []Instr{{Op: OpPushLocal}}
		var debug []core.SrcPos
		var tbl map[int]LandingWord
		lw := &lowerer{es: es, code: &code, debug: &debug, landingWords: &tbl}
		lw.seatLandingWord(LandingWord{Name: "z"}, 1)
		code = append(code, Instr{Op: OpReStepLanding, Arg: 3}, word, Instr{Op: OpCallDynamic, Arg: 1})
		return lw, tbl
	}
	landed, arg := EventOperand(1, 0), EventOperand(2, 0)

	lw, tbl := build(Instr{Op: OpCallUser, Arg: 0}, userCall)
	lw.sealLandingSkip(OpCallDynamic, []EmitOperand{landed, arg})
	if got := tbl[1].Skip; got != 4 {
		t.Errorf("a compiled fn word's CALL_USER: the claim resumes past the apply (pc 4), got %d", got)
	}
	for _, op := range []Opcode{OpCallNative, OpCallNativePoly} {
		lw, tbl = build(Instr{Op: op}, nativeCall)
		lw.sealLandingSkip(OpCallDynamic, []EmitOperand{landed, arg})
		if got := tbl[1].Skip; got != 4 {
			t.Errorf("a native word's %v: the claim target, got %d", op, got)
		}
	}

	// The shapes that seat nothing.
	cases := []struct {
		name  string
		word  Instr
		ev    EmitEvent
		dynOp Opcode
		ops   []EmitOperand
	}{
		{"another apply op", Instr{Op: OpCallUser}, userCall, OpCallDynamicTrailing, []EmitOperand{landed, arg}},
		{"a wider residual", Instr{Op: OpCallUser}, userCall, OpCallDynamic, []EmitOperand{landed, arg, arg}},
		{"the fn operand is not the landed value", Instr{Op: OpCallUser}, userCall, OpCallDynamic, []EmitOperand{EventOperand(3, 0), arg}},
		{"the landed value's second result", Instr{Op: OpCallUser}, userCall, OpCallDynamic, []EmitOperand{EventOperand(1, 1), arg}},
		{"a promoted landed value", Instr{Op: OpCallUser}, userCall, OpCallDynamic, []EmitOperand{localOperand(0), arg}},
		{"a const argument", Instr{Op: OpCallUser}, userCall, OpCallDynamic, []EmitOperand{landed, ConstOperand(0)}},
		{"an argument no event of the frame produced", Instr{Op: OpCallUser}, userCall, OpCallDynamic, []EmitOperand{landed, EventOperand(9, 0)}},
		{"another unit", Instr{Op: OpCallUser, Arg: 1}, userCall, OpCallDynamic, []EmitOperand{landed, arg}},
		{"a unit of another name", Instr{Op: OpCallUser}, EmitEvent{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}}, OpCallDynamic, nil},
		{"a word that collects", Instr{Op: OpCallUser}, EmitEvent{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1, ops: []EmitOperand{ConstOperand(0)}}}, OpCallDynamic, []EmitOperand{landed, arg}},
		{"two results", Instr{Op: OpCallUser}, EmitEvent{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 2}}, OpCallDynamic, []EmitOperand{landed, arg}},
		{"a poly user call", Instr{Op: OpCallUser}, EmitEvent{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1, poly: &emitUserPolySpec{}}}, OpCallDynamic, []EmitOperand{landed, arg}},
		{"a routed user call", Instr{Op: OpCallUser}, EmitEvent{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1, generic: true}}, OpCallDynamic, []EmitOperand{landed, arg}},
		{"a unit out of range", Instr{Op: OpCallUser, Arg: 5}, EmitEvent{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 5, nout: 1}}, OpCallDynamic, []EmitOperand{landed, arg}},
		{"a native under another word", Instr{Op: OpCallNative}, EmitEvent{seq: 2, kind: evCall, call: emitCall{word: "y", nout: 1}}, OpCallDynamic, []EmitOperand{landed, arg}},
		{"a native call event lowered to another op", Instr{Op: OpMakeList}, nativeCall, OpCallDynamic, []EmitOperand{landed, arg}},
		{"a native that collects", Instr{Op: OpCallNative}, EmitEvent{seq: 2, kind: evCall, call: emitCall{word: "z", nout: 1, ops: []EmitOperand{ConstOperand(0)}}}, OpCallDynamic, []EmitOperand{landed, arg}},
		{"a native with no result", Instr{Op: OpCallNative}, EmitEvent{seq: 2, kind: evCall, call: emitCall{word: "z"}}, OpCallDynamic, []EmitOperand{landed, arg}},
		{"another event kind", Instr{Op: OpCallUser}, EmitEvent{seq: 2, kind: evStore}, OpCallDynamic, []EmitOperand{landed, arg}},
	}
	for _, c := range cases {
		lw, tbl := build(c.word, c.ev)
		if c.ops == nil {
			lw.es.fnRecs[0].name = "w"
			c.ops = []EmitOperand{landed, arg}
		}
		lw.sealLandingSkip(c.dynOp, c.ops)
		if got := tbl[1].Skip; got != 0 {
			t.Errorf("%s: no claim target, got %d", c.name, got)
		}
	}

	// No landing where the apply looks for one, no table, too little code.
	lw, tbl = build(Instr{Op: OpCallUser}, userCall)
	*lw.code = append(*lw.code, Instr{Op: OpCallDynamic, Arg: 1})
	lw.sealLandingSkip(OpCallDynamic, []EmitOperand{landed, arg})
	if tbl[1].Skip != 0 || len(tbl) != 1 {
		t.Errorf("an apply not two ops past the landing seals nothing: %v", tbl)
	}
	var none []Instr
	(&lowerer{es: NewEmitState(), code: &none}).sealLandingSkip(OpCallDynamic, []EmitOperand{landed, arg})
	short := []Instr{{Op: OpCallDynamic, Arg: 1}}
	var tblS map[int]LandingWord
	(&lowerer{es: NewEmitState(), code: &short, landingWords: &tblS}).sealLandingSkip(OpCallDynamic, []EmitOperand{landed, arg})
	if tblS != nil {
		t.Errorf("too little code seats nothing: %v", tblS)
	}
}
