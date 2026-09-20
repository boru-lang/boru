package modules

import (
	"regexp"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// s7bMap returns an empty concrete opts map.
func s7bMap() native.Value { return native.NewMap(native.NewOrderedMap()) }

// TestS7B_MiniByteLiteralSrcErrors drives the src / subject AsConcreteString
// decline arms of the hb, bb and re handlers (a non-concrete String passes
// the sig but is declined by the handler).
func TestS7B_MiniByteLiteralSrcErrors(t *testing.T) {
	r := mcovReg(t)
	litS := native.NewTypeLiteral(native.TString)

	if _, err := miniHexBytesHandler([]native.Value{litS, s7bMap()}, nil, nil, r); err == nil {
		t.Error("hb: non-concrete src should error")
	}
	if _, err := miniBinBytesHandler([]native.Value{litS, s7bMap()}, nil, nil, r); err == nil {
		t.Error("bb: non-concrete src should error")
	}
	if _, err := miniReHandler([]native.Value{litS, s7bMap(), native.NewString("x")}, nil, nil, r); err == nil {
		t.Error("re: non-concrete src should error")
	}
	// re subject arm: valid src, non-concrete subject.
	if _, err := miniReHandler([]native.Value{native.NewString("a"), s7bMap(), litS}, nil, nil, r); err == nil {
		t.Error("re: non-concrete subject should error")
	}
}

// TestS7B_MiniRunReSubjectError drives run-re's subject-error arm: a valid
// precompiled regexp carrier with a non-concrete subject.
func TestS7B_MiniRunReSubjectError(t *testing.T) {
	r := mcovReg(t)
	tMini := r.Types.MintType("S7bCompiled", native.TIdeal)
	ext := core.NewExtension(tMini, regexp.MustCompile("a"))
	args := []native.Value{ext, s7bMap(), native.NewTypeLiteral(native.TString)}
	if _, err := miniRunReHandler(args, nil, nil, r); err == nil {
		t.Error("run-re: non-concrete subject should error")
	}
}

// TestS7B_MiniBfHandlerErrors drives miniBfHandler's src and input error
// arms.
func TestS7B_MiniBfHandlerErrors(t *testing.T) {
	r := mcovReg(t)
	litS := native.NewTypeLiteral(native.TString)
	if _, err := miniBfHandler([]native.Value{litS, s7bMap()}, nil, nil, r); err == nil {
		t.Error("bf: non-concrete src should error")
	}
	// 3-arg form with a non-concrete stack input.
	if _, err := miniBfHandler([]native.Value{native.NewString("+."), s7bMap(), litS}, nil, nil, r); err == nil {
		t.Error("bf: non-concrete input should error")
	}
}

// TestS7B_RunBrainfuckPointerAndSkip drives runBrainfuck's `>` overrun
// (pointer past the tape end) and the `[`-skip-when-zero arm.
func TestS7B_RunBrainfuckPointerAndSkip(t *testing.T) {
	if _, bferr := runBrainfuck(strings.Repeat(">", 30000), "", 1_000_000); bferr == nil {
		t.Error("runBrainfuck: 30000 rights should overrun the tape")
	}
	// `[]` at cell 0 skips the loop body via the forward jump.
	out, bferr := runBrainfuck("[]", "", 100)
	if bferr != nil || out != "" {
		t.Errorf("runBrainfuck(`[]`) = %q, %v; want empty, nil", out, bferr)
	}
}

// TestS7B_MiniGexShapeDefault drives miniGexShapeReturns' default arm: a
// subject whose type is neither List, Map nor Scalar (a Function) yields the
// dynamic(Any) shape.
func TestS7B_MiniGexShapeDefault(t *testing.T) {
	dummy := native.NewInteger(0)
	out := miniGexShapeReturns([]native.Value{dummy, dummy, s7bFn()}, nil)
	if len(out) != 1 {
		t.Fatalf("miniGexShapeReturns default: got %d values, want 1", len(out))
	}
}

// (The miniRegisterInstall / miniRegisterCompiledHandler seams died with
// the frozen kind namespace — TestMiniCovRegisterTombstonesDirect drives
// the tombstone handlers that replaced them.)

// (The miniMicronLitValidate / micronSpecMapCheck seams died with the
// MiniLang.micron tombstone.)
