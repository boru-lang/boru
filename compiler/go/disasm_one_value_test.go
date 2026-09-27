package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestDisassembleRuntimeCheckedSingleValue: a computed `do` body's call that
// seats its run as ONE runtime-checked value (SigRef/PolyRef.DynBodyOne —
// dyn_body_one.go) says so in the disassembly, on both call shapes, and a
// plain call of the same word does not.
func TestDisassembleRuntimeCheckedSingleValue(t *testing.T) {
	sig := &core.Signature{Args: []*core.Type{core.TList}}
	p := &Program{
		Code: []Instr{
			{Op: OpCallNative, Arg: 0},
			{Op: OpCallNativePoly, Arg: 0},
			{Op: OpCallNative, Arg: 1},
			{Op: OpCallNativePoly, Arg: 1},
		},
		Sigs:     []SigRef{{Word: "do", Sig: sig, Guard: true, DynBodyOne: true}, {Word: "do", Sig: sig}},
		PolyRefs: []PolyRef{{Word: "do", Arity: 1, DynBodyOne: true}, {Word: "do", Arity: 1}},
	}
	lines := strings.Split(p.Disassemble(), "\n")
	const checked = " [one value, checked]"
	for i, want := range []string{
		"; do (List)" + checked, // the checked seat wins over the guard mark
		"; do/1 (poly)" + checked,
	} {
		if !strings.HasSuffix(lines[i], want) {
			t.Errorf("line %d = %q, want the suffix %q", i, lines[i], want)
		}
	}
	for i := 2; i < 4; i++ {
		if strings.Contains(lines[i], checked) {
			t.Errorf("line %d = %q: a plain call is not a checked seat", i, lines[i])
		}
	}
}
