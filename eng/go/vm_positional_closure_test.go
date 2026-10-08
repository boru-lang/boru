package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// twoParamClosureProg is a program whose unit 0 is a fn VALUE's two-param
// `[x:Integer y:Integer]` lambda body answering its FIRST param, and whose
// unit 1 is a callback body unit answering its first slot.
func twoParamClosureProg() *compiler.Program {
	return &compiler.Program{
		Fns: []compiler.CompiledFn{{
			Name: "fnval$body", Lambda: true, NArgs: 2, NParams: 2, NLocals: 2,
			Params: []*core.Type{core.TInteger, core.TInteger},
			Code:   []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug:  []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}, {
			Name: "each$body", NArgs: 1, NParams: 1, NLocals: 1,
			Code:  []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug: []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
}

// TestInvokeClosurePositional pins NUR179: the fn-value-call ops build
// their window in POSITIONAL order (args[0] → the first param), while the
// token seam takes STACK order and matches a fn-value closure top-down
// itself. Handing the seam a positional window bound a two-param closure's
// first param to the value farthest from it; invokeClosurePositional
// re-stacks the window so the seam's own reversal lands args[0] on the
// first param. A callback body unit's closure takes the seam's positional
// binding as it is.
func TestInvokeClosurePositional(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	fnv := compiler.NewClosure(twoParamClosureProg(), 0, nil)
	args := []core.Value{core.NewInteger(5), core.NewInteger(9)}

	res, err := vc.invokeClosurePositional(r, fnv, args)
	if err != nil || len(res) != 1 {
		t.Fatalf("a matching fn-value closure runs its unit: err=%v res=%v", err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 5 {
		t.Errorf("positional: args[0] binds the first param, got %v", res[0])
	}
	res, err = vc.invokeClosure(r, fnv, args)
	if err != nil || len(res) != 1 {
		t.Fatalf("the token seam runs the same unit: err=%v res=%v", err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 9 {
		t.Errorf("the token seam is stack order: the TOP (last) value binds the first param, got %v", res[0])
	}

	body := compiler.NewClosure(twoParamClosureProg(), 1, nil)
	res, err = vc.invokeClosurePositional(r, body, []core.Value{core.NewInteger(7)})
	if err != nil || len(res) != 1 {
		t.Fatalf("a callback body unit's closure runs positionally as it is: err=%v res=%v", err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 7 {
		t.Errorf("the body unit answers its slot, got %v", res[0])
	}
}

// TestLoneTokenArity pins the frame replay's prefix-collecting entry gate:
// a fn value with exactly one own signature has a definite arity, a
// compiled closure with a known unit its params less its captures; a value
// that is no fn, an overloaded one (which matches by signature order
// against the stack), or a closure of an unknown unit declines.
func TestLoneTokenArity(t *testing.T) {
	vc, _ := foreignVC(t, oneConstProg(1))
	one := core.NewValueRaw(core.TFunction, core.FnDefInfo{Name: "inc", Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TInteger}}}}})
	if n, ok := vc.loneTokenArity(one); !ok || n != 1 {
		t.Errorf("one signature of one param: (%d, %v)", n, ok)
	}
	two := core.NewValueRaw(core.TFunction, core.FnDefInfo{Name: "z", Signatures: []core.Signature{{}, {Params: []core.FnParam{{Type: core.TInteger}}}}})
	if _, ok := vc.loneTokenArity(two); ok {
		t.Error("an overloaded fn has no lone arity")
	}
	if _, ok := vc.loneTokenArity(core.NewInteger(5)); ok {
		t.Error("data has no arity")
	}
	cl := compiler.NewClosure(twoParamClosureProg(), 0, nil)
	if n, ok := vc.loneTokenArity(cl); !ok || n != 2 {
		t.Errorf("a closure's unit params: (%d, %v)", n, ok)
	}
	lost := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: twoParamClosureProg(), Unit: 9})
	if _, ok := vc.loneTokenArity(lost); ok {
		t.Error("a closure of an unknown unit declines")
	}
	// The lone-token apply over a prefix: the closure binds its params from
	// the inputs top-down (the token seam's order); data answers ran=false.
	res, err, ran := vc.invokeLoneToken(nil, cl, []core.Value{core.NewInteger(5), core.NewInteger(9)})
	if !ran || err != nil || len(res) != 1 {
		t.Fatalf("a matching closure runs: ran=%v err=%v res=%v", ran, err, res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 9 {
		t.Errorf("the top input binds the first param, got %v", res[0])
	}
}
