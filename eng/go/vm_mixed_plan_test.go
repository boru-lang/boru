package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_mixed_plan_test.go pins mixedPlanApply (vm_mixed_plan.go): the
// interpreter's own plan binds a single-signature, named-param boru fn value
// across a mixed window — the forward token to param 0, the stack beneath to
// param 1 — the value runs on its unit, and what it does not take stays where
// the island leaves it; every window the file comment excludes stays the
// island's. The corpus parity pins are lang's (census_items3_test.go).

// firstParamProg is a program whose one unit takes two params and returns the
// first — enough to read back which value the plan bound to param 0.
func firstParamProg() *compiler.Program {
	return &compiler.Program{
		Fns: []compiler.CompiledFn{{
			Name:    "first",
			NParams: 2,
			NLocals: 2,
			Code:    []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug:   []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
}

func mixedSig(ref *compiler.CompiledFnRef, returns []*core.Type, params ...core.FnParam) core.Signature {
	return core.Signature{Params: params, BarrierPos: -1, Returns: returns, Impl: core.NewBoruImplCompiled([]core.Value{core.NewInteger(0)}, ref)}
}

func mixedFn(anon bool, sigs ...core.Signature) core.Value {
	return core.NewFunction(core.FnDefInfo{Anonymous: anon, Signatures: sigs})
}

func TestMixedPlanApplyBindsTheSplit(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	ref := &compiler.CompiledFnRef{Prog: firstParamProg(), Unit: 0}
	three, two := core.NewInteger(3), core.NewInteger(2)
	for _, anon := range []bool{false, true} {
		fn := mixedFn(anon, mixedSig(ref, nil, intParam("a"), intParam("b")))
		// `3 fn 2`: the forward 2 fills a, the stack's 3 fills b.
		res, err, ran := vc.mixedPlanApply(r, []core.Value{three, fn, two})
		if !ran || err != nil || !sameInts(intsOf(t, res), []int64{2}) {
			t.Fatalf("anon=%v: ran=%v err=%v res=%v", anon, ran, err, res)
		}
		// `9 3 fn 2`: the untaken 9 stays beneath.
		res, err, ran = vc.mixedPlanApply(r, []core.Value{core.NewInteger(9), three, fn, two})
		if !ran || err != nil || !sameInts(intsOf(t, res), []int64{9, 2}) {
			t.Fatalf("anon=%v with a value beneath: ran=%v err=%v res=%v", anon, ran, err, res)
		}
		// `3 fn 2 7 8`: both params fill forward (a=2, b=7), the 3 stays
		// beneath and the untaken 8 lands after the result.
		res, err, ran = vc.mixedPlanApply(r, []core.Value{three, fn, two, core.NewInteger(7), core.NewInteger(8)})
		if !ran || err != nil || !sameInts(intsOf(t, res), []int64{3, 2, 8}) {
			t.Fatalf("anon=%v forward-filled: ran=%v err=%v res=%v", anon, ran, err, res)
		}
	}
}

func TestMixedPlanApplyContractError(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	ref := &compiler.CompiledFnRef{Prog: firstParamProg(), Unit: 0}
	fn := mixedFn(false, mixedSig(ref, []*core.Type{core.TString}, intParam("a"), intParam("b")))
	if _, err, ran := vc.mixedPlanApply(r, []core.Value{core.NewInteger(3), fn, core.NewInteger(2)}); !ran || err == nil {
		t.Fatalf("the value's return contract raises: ran=%v err=%v", ran, err)
	}
}

func TestMixedPlanApplyDeclines(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	ref := &compiler.CompiledFnRef{Prog: firstParamProg(), Unit: 0}
	three, two := core.NewInteger(3), core.NewInteger(2)
	good := mixedFn(false, mixedSig(ref, nil, intParam("a"), intParam("b")))
	quoted := good
	quoted.Quoted = true
	list := core.NewList([]core.Value{three})
	unnamed := core.FnParam{Type: core.TInteger}
	native := core.Signature{Params: []core.FnParam{intParam("a"), intParam("b")}, BarrierPos: -1, Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, nil
	})}
	unstamped := mixedFn(false, core.Signature{Params: []core.FnParam{intParam("a"), intParam("b")}, BarrierPos: -1, Impl: core.Boru([]core.Value{three})})
	for _, c := range []struct {
		label  string
		window []core.Value
	}{
		{"a second value the island steps", []core.Value{three, good, list}},
		{"a non-fn value interior", []core.Value{three, list, two}},
		{"a quoted fn is data", []core.Value{three, quoted, two}},
		{"two signatures", []core.Value{three, mixedFn(false, mixedSig(ref, nil, intParam("a"), intParam("b")), mixedSig(ref, nil, strParam("a"))), two}},
		{"an unnamed param", []core.Value{three, mixedFn(false, mixedSig(ref, nil, intParam("a"), unnamed)), two}},
		{"a zero-argument signature", []core.Value{three, mixedFn(false, mixedSig(ref, nil)), two}},
		{"a native", []core.Value{three, mixedFn(false, native), two}},
		{"no pick: the stack value is rejected", []core.Value{core.NewString("x"), good, two}},
		{"no unit to run", []core.Value{three, unstamped, two}},
	} {
		if _, _, ran := vc.mixedPlanApply(r, c.window); ran {
			t.Errorf("%s must keep the island", c.label)
		}
	}
}

// contiguousHalves accepts only the collection kernel's layout: the values
// right beneath the pointer and the tokens right after it.
func TestContiguousHalves(t *testing.T) {
	if s, f, ok := contiguousHalves([]int{3, 1}, 2); !ok || s != 1 || f != 1 {
		t.Fatalf("one after, one beneath: %d %d %v", s, f, ok)
	}
	if _, _, ok := contiguousHalves([]int{4, 1}, 2); ok {
		t.Fatal("a forward token skipped over is not a contiguous half")
	}
	if _, _, ok := contiguousHalves([]int{0}, 2); ok {
		t.Fatal("a stack value beneath an untaken one is not a contiguous half")
	}
	if _, _, ok := contiguousHalves([]int{2}, 2); ok {
		t.Fatal("the pointer itself is no argument")
	}
}
