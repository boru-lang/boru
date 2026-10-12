package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestCallDynTrailKeepQQuotedStaysData pins OpCallDynTrailKeepQ's quoted
// arm: an event-provenance fn whose RUNTIME value is quoted stays inert —
// the [args, fn] window is exactly the interpreter's residual for a quoted
// call result, so the op leaves the stack untouched (no strip; the
// unquoted path shares callDynTrailTop's body and is pinned end-to-end by
// frontier-hof-audit.tsv §9c's `(2 (mk 4))` row).
func TestCallDynTrailKeepQQuotedStaysData(t *testing.T) {
	qfn := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
	}}})
	core.SetQuoted(&qfn, true)
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(2), qfn},
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},
			{Op: compiler.OpPushConst, Arg: 1},
			{Op: compiler.OpCallDynTrailKeepQ, Arg: 1},
		},
		Debug: make([]core.SrcPos, 3),
	}
	r := runUnitReg(t)
	res, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("keepQ quoted run: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("a quoted fn must stay data ([arg, fn] residual), got %d values: %v", len(res), res)
	}
	if n, _ := core.AsInteger(res[0]); n != 2 {
		t.Errorf("the arg below the quoted fn must survive untouched, got %v", res[0])
	}
	if !core.IsQuoted(&res[1]) {
		t.Error("the quoted fn's quote state must survive the op")
	}
}
