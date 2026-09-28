package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// cover_pr505_lower_test.go pins, on constructed state, arms the
// reverse-order NUR run added to the lowering, the disassembler and the
// static-index fold: the defensive guards no suite's program trips — a
// recorder defect, a shape the recording arms already refuse, a missing
// piece of state, a token with no source position — and the landing
// skip's layout rule, stated here as the unit's own contract. The program-reachable arms of the same run are pinned on both
// lanes in lang/go/cover_pr505_lower_test.go.

// TestDisassembleNamesASignaturelessNativeCall: a CALL_NATIVE whose
// signature entry carries no signature — a recorder defect, the one NUR162
// measured under a paren-grouped `word` bound to a fn value — disassembles
// by name instead of dereferencing the missing signature.
func TestDisassembleNamesASignaturelessNativeCall(t *testing.T) {
	p := &Program{Code: []Instr{{Op: OpCallNative, Arg: 0}}, Sigs: []SigRef{{Word: "dbl"}}}
	out := p.Disassemble()
	if !strings.Contains(out, "CALL_NATIVE s0   ; dbl (NO SIGNATURE — recorder defect)") {
		t.Errorf("a signature-less entry must be named, not dereferenced:\n%s", out)
	}
}

// TestIsFrameArgsList pins the static-index fold's `args` test: only the
// CURRENT frame's args list is it, and every missing piece — no recorder, a
// recorder bound to no registry, a registry with no args stack, a value
// with no identity to compare — answers false rather than dereferencing.
func TestIsFrameArgsList(t *testing.T) {
	frame := core.NewList(nil)
	frame.ID = "args-frame"
	other := core.NewList(nil)
	other.ID = "other-list"
	stack := core.NewArgsStack()
	if err := stack.Push(frame); err != nil {
		t.Fatal(err)
	}
	bound := NewEmitState()
	bound.reg = &core.Registry{Args: stack}
	noArgs := NewEmitState()
	noArgs.reg = &core.Registry{}
	for _, c := range []struct {
		name string
		es   *EmitState
		v    core.Value
		want bool
	}{
		{"no recorder", nil, frame, false},
		{"no registry", NewEmitState(), frame, false},
		{"no args stack", noArgs, frame, false},
		{"a value with no identity", bound, core.NewList(nil), false},
		{"another list", bound, other, false},
		{"the frame's own args list", bound, frame, true},
	} {
		if got := isFrameArgsList(c.es, c.v); got != c.want {
			t.Errorf("%s: isFrameArgsList = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestZeroArgDynApplyDeclines pins lowerCall's belt for NUR162: a dyn-apply
// record over NO argument has no signature to lower under — its SigRef
// would carry none, the crash NUR162 measured — and the recording arms
// decline it (RecordDynApplyName, the trailing apply's 0-arg callee). A
// record that reaches the lowering anyway declines here and emits nothing.
func TestZeroArgDynApplyDeclines(t *testing.T) {
	cf := &CompiledFn{}
	lw := &lowerer{es: NewEmitState(), p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{}}
	ev := EmitEvent{kind: evCall, seq: 3, call: emitCall{word: wordDynApply, nout: 1}}
	if reason := lw.lowerCall(&ev); reason != "fn-value apply over no argument (no signature to lower under)" {
		t.Fatalf("a 0-arg dyn apply lowered: reason %q", reason)
	}
	if len(cf.Code) != 0 || len(lw.p.Sigs) != 0 {
		t.Errorf("the decline emits nothing: code %v sigs %v", cf.Code, lw.p.Sigs)
	}
}

// TestSeatLandingSkipLayouts pins seatLandingSkip's layout rule (NUR190): a
// walked landing's pending skip is seated by the paren apply over its lead
// only when every op since the landing is the word's ONE call, with at
// most the SWAP laying the lead beneath it — SkipTo past the apply, SkipOut
// the apply's claim. A pushed operand, two calls or none keep the landing's
// loud defer (no skip), and an apply over another lead leaves the pending
// skip where it is.
func TestSeatLandingSkipLayouts(t *testing.T) {
	apply := &emitCall{dynMethod: &DynMethodSpec{NOut: 1}, ops: []EmitOperand{EventOperand(7, 0), ConstOperand(0)}}
	seat := func(pending int, after ...Opcode) (LandingWord, bool) {
		code := []Instr{{Op: OpReStepLanding}}
		for _, op := range after {
			code = append(code, Instr{Op: op})
		}
		words := map[int]LandingWord{0: {Name: "y"}}
		lw := &lowerer{code: &code, landingWords: &words, landingSkips: map[int]int{pending: 0}}
		lw.seatLandingSkip(apply)
		_, still := lw.landingSkips[pending]
		return words[0], still
	}
	if w, still := seat(7, OpCallUser, OpSwap); still || w.SkipTo != 4 || w.SkipOut != 1 {
		t.Errorf("the word's one call: skip past the apply, got %+v (still pending %v)", w, still)
	}
	for _, c := range []struct {
		name  string
		after []Opcode
	}{
		{"a pushed operand", []Opcode{OpPushConst, OpCallNative, OpSwap}},
		{"two calls", []Opcode{OpCallUser, OpCallUser, OpSwap}},
		{"no call", []Opcode{OpSwap}},
	} {
		if w, still := seat(7, c.after...); still || w.SkipTo != 0 {
			t.Errorf("%s: no skip, the pending entry consumed; got %+v (still pending %v)", c.name, w, still)
		}
	}
	if w, still := seat(9, OpCallUser, OpSwap); !still || w.SkipTo != 0 {
		t.Errorf("an apply over another lead leaves the pending skip: got %+v (still pending %v)", w, still)
	}
}

// TestLandingIslandNeedsAPosition pins landingIsland's position guard: a
// word with no source position (Row 0 — a synthesized token) has no island,
// even over a body whose own synthesized tokens carry the same zero
// position, which would otherwise match the first of them.
func TestLandingIslandNeedsAPosition(t *testing.T) {
	body := []core.Value{core.NewWord("y"), core.NewInteger(5)}
	if opens, island, ok := landingIsland(body, core.SrcPos{}); ok || opens != 0 || island != nil {
		t.Errorf("a position-less word: no island, got %d %v %v", opens, island, ok)
	}
	body[0] = deoptTok("y", 4)
	if opens, island, ok := landingIsland(body, deoptAt(4)); !ok || opens != 0 || len(island) != 2 {
		t.Errorf("the word's own token opens the island: got %d %v %v", opens, island, ok)
	}
}
