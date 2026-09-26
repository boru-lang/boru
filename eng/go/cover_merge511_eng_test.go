package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// cover_merge511_eng_test.go pins the VM arms the corpus never reaches (the
// merged ADR-008 gate on the reverse-order NUR run's merge of main's #511),
// each with the verdict the arm exists to give.

// bindRootRead (NUR207): a closure the bridge cannot describe installs
// nothing, and an island that bound the name again keeps its own binding
// when the install is undone.
func TestBindRootReadArms(t *testing.T) {
	r := seam7Reg(t)
	p := &compiler.Program{}
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	bad := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 3, InShape: compiler.ClosureInValue, Ident: core.NewFnIdentity()})
	undo := vc.bindRootRead(r, true, "zzj", bad)
	if r.Defs.Has("zzj") {
		t.Fatal("a closure naming no unit installs nothing")
	}
	undo()
	fn := core.NewFunction(core.FnDefInfo{Name: "zzj", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}})
	undo = vc.bindRootRead(r, true, "zzj", fn)
	if !r.Defs.Has("zzj") {
		t.Fatal("a fn value is installed under its name for the island")
	}
	r.Defs.Push("zzj", core.NewInteger(7)) // the island's own `def zzj 7`
	undo()
	if top, ok := r.Defs.Top("zzj"); !ok || !top.Parent.ConformsTo(core.TInteger) {
		t.Errorf("the island's own binding must survive the undo, got %v", top)
	}
}

// SPLICE_DYN under the recorder's one-value claim (Arg 1): a payload that
// spreads to any other count defers, since the apply after it would lay N
// values out as one.
func TestSpliceDynClaimedOneValueDefersOnMore(t *testing.T) {
	p := &compiler.Program{
		Consts: []core.Value{core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})},
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpSpliceDyn, Arg: 1}},
		Debug:  make([]core.SrcPos, 2),
	}
	if _, err := RunProgram(p, seam7Reg(t)); err == nil || !strings.Contains(err.Error(), "multi-value payload") {
		t.Fatalf("want the multi-value defer, got %v", err)
	}
}

// callDynMethod over a MODIFIER WRAPPER of a compiled closure: the wrapper
// applies the closure itself, and a raise inside it surfaces as the
// method's own error.
func TestCallDynMethodWrapperOverRaisingClosure(t *testing.T) {
	r := seam7Reg(t)
	p := &compiler.Program{
		Traps: []compiler.TrapSpec{{Code: "type_error", Detail: "zz boom", Word: "boom"}},
		Fns: []compiler.CompiledFn{{Name: "boom$body", NParams: 1, NArgs: 1, NLocals: 1, Params: []*core.Type{core.TInteger},
			Code: []compiler.Instr{{Op: compiler.OpTrap, Arg: 0}}, Debug: []core.SrcPos{{}}}},
	}
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 0, InShape: compiler.ClosureInValue, Ident: core.NewFnIdentity()})
	wrapper := core.NewFunction(core.FnDefInfo{Name: "w", Wraps: &cl, Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "x", Type: core.TInteger}}, Args: []*core.Type{core.TInteger}, BarrierPos: 1,
		Impl: core.Boru([]core.Value{core.NewWord("x")}),
	}}})
	spec := &compiler.DynMethodSpec{Word: "f", NArgs: 1, NOut: 1}
	_, _, err := vc.callDynMethod(r, spec, []core.Value{core.NewInteger(5), wrapper}, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "zz boom") {
		t.Fatalf("the wrapped closure's raise is the method's error, got %v", err)
	}
}
