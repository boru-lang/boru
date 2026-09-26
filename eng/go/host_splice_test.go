package eng

import (
	"errors"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// host_splice_test.go covers the VM half of the hosted splice
// (compiler.SigRef.HostSplice, 2026-09-26): a CALL_NATIVE whose handler
// returns the interpreter's SPLICE — a computed `for` body's loop tokens —
// runs those tokens on the program's interpreter island instead of rejecting
// them as a tape-coupled result. A hand-built program drives it with a
// handler returning an active token window.

// spliceSig is a 0-arg native returning toks verbatim, as a structured
// word's handler returns its splice.
func spliceSig(toks ...core.Value) *core.Signature {
	return &core.Signature{
		BarrierPos: -1,
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return toks, nil
		}),
	}
}

func hostSpliceProgram(sig *core.Signature, host bool) *compiler.Program {
	return &compiler.Program{
		Sigs:  []compiler.SigRef{{Word: "zz-splice", Sig: sig, HostSplice: host}},
		Code:  []compiler.Instr{{Op: compiler.OpCallNative, Arg: 0}},
		Debug: []core.SrcPos{{Row: 1, Col: 1}},
	}
}

func TestHostSpliceRunsTheHandlersTokensOnTheIsland(t *testing.T) {
	// A paren group: tape-coupled tokens only an engine can step.
	window := []core.Value{core.NewOpenParen(), core.NewInteger(5), core.NewCloseParen()}
	res, err := RunProgram(hostSpliceProgram(spliceSig(window...), true), runUnitReg(t))
	if err != nil {
		t.Fatalf("hosted splice: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("hosted splice: want the island's one result, got %v", res)
	}
	if n, _ := core.AsInteger(res[0]); n != 5 {
		t.Fatalf("hosted splice: `( 5 )` on the island is 5, got %v", res[0])
	}
	// The same result on an unhosted call site is the tape-coupled result
	// the VM refuses: the flag is the call site's, never the signature's.
	if _, err := RunProgram(hostSpliceProgram(spliceSig(window...), false), runUnitReg(t)); err == nil || !strings.Contains(err.Error(), "tape-coupled") {
		t.Fatalf("unhosted splice must be refused as tape-coupled, got %v", err)
	}
}

func TestHostSpliceIslandErrorIsTheInterpretersOwn(t *testing.T) {
	sig := spliceSig(core.NewWord("zz-no-such-word"))
	_, err := RunProgram(hostSpliceProgram(sig, true), runUnitReg(t))
	var ae *core.BoruError
	if !errors.As(err, &ae) || ae.Code != "undefined_word" {
		t.Fatalf("an island error surfaces as the interpreter raised it, got %v", err)
	}
}
